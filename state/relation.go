// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"sort"
	"strings"

	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// relationKey returns a string describing the relation defined by
// endpoints, for use in various contexts (including error messages).
func relationKey(endpoints []Endpoint) string {
	eps := epSlice{}
	for _, ep := range endpoints {
		eps = append(eps, ep)
	}
	sort.Sort(eps)
	names := []string{}
	for _, ep := range eps {
		names = append(names, ep.String())
	}
	return strings.Join(names, " ")
}

// relationDoc is the internal representation of a Relation in MongoDB.
// Note the correspondence with RelationInfo in apiserver/params.
type relationDoc struct {
	DocID     string `bson:"_id"`
	Key       string `bson:"key"`
	ModelUUID string `bson:"model-uuid"`
	Id        int
	Endpoints []Endpoint
	Life      Life
	UnitCount int
}

// Relation represents a relation between one or two service endpoints.
type Relation struct {
	st  *State
	doc relationDoc
}

func newRelation(st *State, doc *relationDoc) *Relation {
	return &Relation{
		st:  st,
		doc: *doc,
	}
}

func (r *Relation) String() string {
	return r.doc.Key
}

// Tag returns a name identifying the relation.
func (r *Relation) Tag() names.Tag {
	return names.NewRelationTag(r.doc.Key)
}

// Refresh refreshes the contents of the relation from the underlying
// state. It returns an error that satisfies errors.IsNotFound if the
// relation has been removed.
func (r *Relation) Refresh() error {
	relations, closer := r.st.getCollection(relationsC)
	defer closer()

	doc := relationDoc{}
	err := relations.FindId(r.doc.DocID).One(&doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("relation %v", r)
	}
	if err != nil {
		return errors.Annotatef(err, "cannot refresh relation %v", r)
	}
	if r.doc.Id != doc.Id {
		// The relation has been destroyed and recreated. This is *not* the
		// same relation; if we pretend it is, we run the risk of violating
		// the lifecycle-only-advances guarantee.
		return errors.NotFoundf("relation %v", r)
	}
	r.doc = doc
	return nil
}

// Life returns the relation's current life state.
func (r *Relation) Life() Life {
	return r.doc.Life
}

// Destroy ensures that the relation will be removed at some point; if no units
// are currently in scope, it will be removed immediately.
func (r *Relation) Destroy() (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot destroy relation %q", r)
	if len(r.doc.Endpoints) == 1 && r.doc.Endpoints[0].Role == charm.RolePeer {
		return errors.Errorf("is a peer relation")
	}
	defer func() {
		if err == nil {
			// This is a white lie; the document might actually be removed.
			r.doc.Life = Dying
		}
	}()
	rel := &Relation{r.st, r.doc}
	// In this context, aborted transactions indicate that the number of units
	// in scope have changed between 0 and not-0. The chances of 5 successive
	// attempts each hitting this change -- which is itself an unlikely one --
	// are considered to be extremely small.
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := rel.Refresh(); errors.IsNotFound(err) {
				return []txn.Op{}, nil
			} else if err != nil {
				return nil, err
			}
		}
		ops, _, err := rel.destroyOps("")
		if err == errAlreadyDying {
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, err
		}
		return ops, nil
	}
	return rel.st.run(buildTxn)
}

// destroyOps returns the operations necessary to destroy the relation, and
// whether those operations will lead to the relation's removal. These
// operations may include changes to the relation's services; however, if
// ignoreService is not empty, no operations modifying that service will
// be generated.
func (r *Relation) destroyOps(ignoreService string) (ops []txn.Op, isRemove bool, err error) {
	if r.doc.Life != Alive {
		return nil, false, errAlreadyDying
	}
	if r.doc.UnitCount == 0 {
		removeOps, err := r.removeOps(ignoreService, "")
		if err != nil {
			return nil, false, err
		}
		return removeOps, true, nil
	}
	return []txn.Op{{
		C:      relationsC,
		Id:     r.doc.DocID,
		Assert: bson.D{{"life", Alive}, {"unitcount", bson.D{{"$gt", 0}}}},
		Update: bson.D{{"$set", bson.D{{"life", Dying}}}},
	}}, false, nil
}

// removeOps returns the operations necessary to remove the relation. If
// ignoreService is not empty, no operations affecting that service will be
// included; if departingUnitName is non-empty, this implies that the
// relation's services may be Dying and otherwise unreferenced, and may thus
// require removal themselves.
func (r *Relation) removeOps(ignoreService string, departingUnitName string) ([]txn.Op, error) {
	relOp := txn.Op{
		C:      relationsC,
		Id:     r.doc.DocID,
		Remove: true,
	}
	if departingUnitName != "" {
		relOp.Assert = bson.D{{"life", Dying}, {"unitcount", 1}}
	} else {
		relOp.Assert = bson.D{{"life", Alive}, {"unitcount", 0}}
	}
	ops := []txn.Op{relOp}
	for _, ep := range r.doc.Endpoints {
		if ep.ApplicationName == ignoreService {
			continue
		}
		app, err := applicationByName(r.st, ep.ApplicationName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if app.IsRemote() {
			epOps, err := r.removeRemoteEndpointOps(ep, departingUnitName != "")
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, epOps...)
		} else {
			epOps, err := r.removeLocalEndpointOps(ep, departingUnitName)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, epOps...)
		}
	}
	cleanupOp := newCleanupOp(cleanupRelationSettings, fmt.Sprintf("r#%d#", r.Id()))
	return append(ops, cleanupOp), nil
}

func (r *Relation) removeLocalEndpointOps(ep Endpoint, departingUnitName string) ([]txn.Op, error) {
	var asserts bson.D
	hasRelation := bson.D{{"relationcount", bson.D{{"$gt", 0}}}}
	departingUnitServiceMatchesEndpoint := func() bool {
		s, err := names.UnitApplication(departingUnitName)
		return err == nil && s == ep.ApplicationName
	}
	if departingUnitName == "" {
		// We're constructing a destroy operation, either of the relation
		// or one of its services, and can therefore be assured that both
		// services are Alive.
		asserts = append(hasRelation, isAliveDoc...)
	} else if departingUnitServiceMatchesEndpoint() {
		// This service must have at least one unit -- the one that's
		// departing the relation -- so it cannot be ready for removal.
		cannotDieYet := bson.D{{"unitcount", bson.D{{"$gt", 0}}}}
		asserts = append(hasRelation, cannotDieYet...)
	} else {
		// This service may require immediate removal.
		applications, closer := r.st.getCollection(applicationsC)
		defer closer()

		svc := &Application{st: r.st}
		hasLastRef := bson.D{{"life", Dying}, {"unitcount", 0}, {"relationcount", 1}}
		removable := append(bson.D{{"_id", ep.ApplicationName}}, hasLastRef...)
		if err := applications.Find(removable).One(&svc.doc); err == nil {
			return svc.removeOps(hasLastRef)
		} else if err != mgo.ErrNotFound {
			return nil, err
		}
		// If not, we must check that this is still the case when the
		// transaction is applied.
		asserts = bson.D{{"$or", []bson.D{
			{{"life", Alive}},
			{{"unitcount", bson.D{{"$gt", 0}}}},
			{{"relationcount", bson.D{{"$gt", 1}}}},
		}}}
	}
	return []txn.Op{{
		C:      applicationsC,
		Id:     r.st.docID(ep.ApplicationName),
		Assert: asserts,
		Update: bson.D{{"$inc", bson.D{{"relationcount", -1}}}},
	}}, nil
}

func (r *Relation) removeRemoteEndpointOps(ep Endpoint, unitDying bool) ([]txn.Op, error) {
	var asserts bson.D
	hasRelation := bson.D{{"relationcount", bson.D{{"$gt", 0}}}}
	if !unitDying {
		// We're constructing a destroy operation, either of the relation
		// or one of its services, and can therefore be assured that both
		// services are Alive.
		asserts = append(hasRelation, isAliveDoc...)
	} else {
		// The remote application may require immediate removal.
		services, closer := r.st.getCollection(remoteApplicationsC)
		defer closer()

		svc := &RemoteApplication{st: r.st}
		hasLastRef := bson.D{{"life", Dying}, {"relationcount", 1}}
		removable := append(bson.D{{"_id", ep.ApplicationName}}, hasLastRef...)
		if err := services.Find(removable).One(&svc.doc); err == nil {
			return svc.removeOps(hasLastRef), nil
		} else if err != mgo.ErrNotFound {
			return nil, err
		}
		// If not, we must check that this is still the case when the
		// transaction is applied.
		asserts = bson.D{{"$or", []bson.D{
			{{"life", Alive}},
			{{"relationcount", bson.D{{"$gt", 1}}}},
		}}}
	}
	return []txn.Op{{
		C:      remoteApplicationsC,
		Id:     r.st.docID(ep.ApplicationName),
		Assert: asserts,
		Update: bson.D{{"$inc", bson.D{{"relationcount", -1}}}},
	}}, nil
}

// Id returns the integer internal relation key. This is exposed
// because the unit agent needs to expose a value derived from this
// (as JUJU_RELATION_ID) to allow relation hooks to differentiate
// between relations with different services.
func (r *Relation) Id() int {
	return r.doc.Id
}

// Endpoint returns the endpoint of the relation for the named service.
// If the service is not part of the relation, an error will be returned.
func (r *Relation) Endpoint(applicationname string) (Endpoint, error) {
	for _, ep := range r.doc.Endpoints {
		if ep.ApplicationName == applicationname {
			return ep, nil
		}
	}
	return Endpoint{}, errors.Errorf("application %q is not a member of %q", applicationname, r)
}

// Endpoints returns the endpoints for the relation.
func (r *Relation) Endpoints() []Endpoint {
	return r.doc.Endpoints
}

// RelatedEndpoints returns the endpoints of the relation r with which
// units of the named service will establish relations. If the service
// is not part of the relation r, an error will be returned.
func (r *Relation) RelatedEndpoints(applicationname string) ([]Endpoint, error) {
	local, err := r.Endpoint(applicationname)
	if err != nil {
		return nil, err
	}
	role := counterpartRole(local.Role)
	var eps []Endpoint
	for _, ep := range r.doc.Endpoints {
		if ep.Role == role {
			eps = append(eps, ep)
		}
	}
	if eps == nil {
		return nil, errors.Errorf("no endpoints of %q relate to application %q", r, applicationname)
	}
	return eps, nil
}

// Unit returns a RelationUnit for the supplied unit.
func (r *Relation) Unit(u *Unit) (*RelationUnit, error) {
	const checkUnitLife = true
	return r.unit(u.Name(), u.doc.Principal, u.IsPrincipal(), checkUnitLife)
}

// RemoteUnit returns a RelationUnit for the supplied unit
// of a remote application.
func (r *Relation) RemoteUnit(unitName string) (*RelationUnit, error) {
	// Verify that the unit belongs to a remote application.
	serviceName, err := names.UnitApplication(unitName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if _, err := r.st.RemoteApplication(serviceName); err != nil {
		return nil, errors.Trace(err)
	}
	// Only non-subordinate services may be offered for remote
	// relation, so all remote units are principals.
	const principal = ""
	const isPrincipal = true
	const checkUnitLife = false
	return r.unit(unitName, principal, isPrincipal, checkUnitLife)
}

func (r *Relation) unit(
	unitName string,
	principal string,
	isPrincipal bool,
	checkUnitLife bool,
) (*RelationUnit, error) {
	serviceName, err := names.UnitApplication(unitName)
	if err != nil {
		return nil, err
	}
	ep, err := r.Endpoint(serviceName)
	if err != nil {
		return nil, err
	}
	scope := r.globalScope()
	if ep.Scope == charm.ScopeContainer {
		container := principal
		if container == "" {
			container = unitName
		}
		scope = fmt.Sprintf("%s#%s", scope, container)
	}
	return &RelationUnit{
		st:            r.st,
		relation:      r,
		unitName:      unitName,
		isPrincipal:   isPrincipal,
		checkUnitLife: checkUnitLife,
		endpoint:      ep,
		scope:         scope,
	}, nil
}

// globalScope returns the scope prefix for relation scope document keys
// in the global scope.
func (r *Relation) globalScope() string {
	return fmt.Sprintf("r#%d", r.doc.Id)
}

// relationSettingsCleanupChange removes the settings doc.
type relationSettingsCleanupChange struct {
	Prefix string
}

// Prepare is part of the Change interface.
func (change relationSettingsCleanupChange) Prepare(db Database) ([]txn.Op, error) {
	settings, closer := db.GetCollection(settingsC)
	defer closer()
	sel := bson.D{{"_id", bson.D{{"$regex", "^" + change.Prefix}}}}
	var docs []struct {
		DocID string `bson:"_id"`
	}
	err := settings.Find(sel).Select(bson.D{{"_id", 1}}).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(docs) == 0 {
		return nil, ErrChangeComplete
	}

	ops := make([]txn.Op, len(docs))
	for i, doc := range docs {
		ops[i] = txn.Op{
			C:      settingsC,
			Id:     doc.DocID,
			Remove: true,
		}
	}
	return ops, nil

}
