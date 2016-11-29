// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"gopkg.in/juju/charm.v6-unstable"
)

// OfferFilterParams contains filters used to query application offers
// from one or more directories.
type OfferFilterParams struct {
	Filters []OfferFilters `json:"filters"`
}

// EndpointFilterAttributes is used to filter offers matching the
// specified endpoint criteria.
type EndpointFilterAttributes struct {
	Role      charm.RelationRole `json:"role"`
	Interface string             `json:"interface"`
	Name      string             `json:"name"`
}

// OfferFilters is used to query offers in an application directory.
// Offers matching any of the filters are returned.
type OfferFilters struct {
	Directory string
	Filters   []OfferFilter
}

// OfferFilter is used to query offers in a application directory.
type OfferFilter struct {
	ApplicationURL         string                     `json:"application-url"`
	SourceLabel            string                     `json:"source-label"`
	SourceModelUUIDTag     string                     `json:"source-uuid"`
	ApplicationName        string                     `json:"application-name"`
	ApplicationDescription string                     `json:"application-description"`
	ApplicationUser        string                     `json:"application-user"`
	Endpoints              []EndpointFilterAttributes `json:"endpoints"`
	AllowedUserTags        []string                   `json:"allowed-users"`
}

// ApplicationOffer represents an application offering from an external model.
type ApplicationOffer struct {
	ApplicationURL         string           `json:"application-url"`
	SourceModelTag         string           `json:"source-model-tag"`
	SourceLabel            string           `json:"source-label"`
	ApplicationName        string           `json:"application-name"`
	ApplicationDescription string           `json:"application-description"`
	Endpoints              []RemoteEndpoint `json:"endpoints"`
}

// AddApplicationOffers is used when adding offers to a application directory.
type AddApplicationOffers struct {
	Offers []AddApplicationOffer
}

// AddApplicationOffer represents a application offering from an external environment.
type AddApplicationOffer struct {
	ApplicationOffer
	// UserTags are those who can consume the offer.
	UserTags []string `json:"users"`
}

// ApplicationOfferResults is a result of listing application offers.
type ApplicationOfferResults struct {
	Offers []ApplicationOffer
	Error  *Error
}

// RemoteEndpoint represents a remote application endpoint.
type RemoteEndpoint struct {
	Name      string              `json:"name"`
	Role      charm.RelationRole  `json:"role"`
	Interface string              `json:"interface"`
	Limit     int                 `json:"limit"`
	Scope     charm.RelationScope `json:"scope"`
}

// ApplicationOfferParams is used to offer remote applications.
type ApplicationOfferParams struct {
	// ModelTag is the tag of the model containing the application to offer.
	ModelTag string `json:"model-tag"`

	// ApplicationURL may contain user supplied application url.
	ApplicationURL string `json:"application-url,omitempty"`

	// ApplicationName contains name of application being offered.
	ApplicationName string `json:"application-name"`

	// ApplicationDescription is description for the offered application.
	// For now, this defaults to description provided in the charm or
	// is supplied by the user.
	ApplicationDescription string `json:"application-description"`

	// Endpoints contains offered application endpoints.
	Endpoints []string `json:"endpoints"`

	// AllowedUserTags contains tags of users that are allowed to use this offered application.
	AllowedUserTags []string `json:"allowed-users"`
}

// ApplicationOffersParams contains a collection of offers to allow adding offers in bulk.
type ApplicationOffersParams struct {
	Offers []ApplicationOfferParams `json:"offers"`
}

// FindApplicationOffersResults is a result of finding remote application offers.
type FindApplicationOffersResults struct {
	// Results contains application offers matching each filter.
	Results []ApplicationOfferResults `json:"results"`
}

// ApplicationOfferResult is a result of listing a remote application offer.
type ApplicationOfferResult struct {
	// Result contains application offer information.
	Result ApplicationOffer `json:"result"`

	// Error contains related error.
	Error *Error `json:"error,omitempty"`
}

// ApplicationOffersResults is a result of listing remote application offers.
type ApplicationOffersResults struct {
	// Result contains collection of remote application results.
	Results []ApplicationOfferResult `json:"results,omitempty"`
}

// ApplicationURLs is a filter used to select remote applications via show call.
type ApplicationURLs struct {
	// ApplicationURLs contains collection of urls for applications that are to be shown.
	ApplicationURLs []string `json:"application-urls,omitempty"`
}

// OfferedApplication represents attributes for an offered application.
type OfferedApplication struct {
	ApplicationURL  string            `json:"application-url"`
	ApplicationName string            `json:"application-name"`
	CharmName       string            `json:"charm-name"`
	Description     string            `json:"description"`
	Registered      bool              `json:"registered"`
	Endpoints       map[string]string `json:"endpoints"`
}

// OfferedApplicationResult holds the result of loading an
// offered application at a URL.
type OfferedApplicationResult struct {
	Result OfferedApplication `json:"result"`
	Error  *Error             `json:"error,omitempty"`
}

// OfferedApplicationResults represents the result of a ListOfferedApplications call.
type OfferedApplicationResults struct {
	Results []OfferedApplicationResult
}

// OfferedApplicationDetails is an application found during a request to list remote applications.
type OfferedApplicationDetails struct {
	// ApplicationURL may contain user supplied application URL.
	ApplicationURL string `json:"application-url,omitempty"`

	// ApplicationName contains name of application being offered.
	ApplicationName string `json:"application-name"`

	// CharmName is the charm name of this service.
	CharmName string `json:"charm-name"`

	// UsersCount is the count of how many users are connected to this shared application.
	UsersCount int `json:"users-count,omitempty"`

	// Endpoints is a list of charm relations that this remote application offered.
	Endpoints []RemoteEndpoint `json:"endpoints"`
}

// OfferedApplicationDetailsResult is a result of listing a remote application.
type OfferedApplicationDetailsResult struct {
	// Result contains remote application information.
	Result *OfferedApplicationDetails `json:"result,omitempty"`

	// Error contains error related to this item.
	Error *Error `json:"error,omitempty"`
}

// ListOffersFilterResults is a result of listing remote application offers
// for an application directory.
type ListOffersFilterResults struct {
	// Error contains error related to this directory.
	Error *Error `json:"error,omitempty"`

	// Result contains collection of remote application item results for this directory.
	Result []OfferedApplicationDetailsResult `json:"result,omitempty"`
}

// ListOffersResults is a result of listing remote application offers
// for application directories.
type ListOffersResults struct {
	// Results contains collection of remote directories results.
	Results []ListOffersFilterResults `json:"results,omitempty"`
}

// OfferedApplicationFilters has sets of filters that
// are used by a vendor to query remote applications that the vendor has offered.
type OfferedApplicationFilters struct {
	Filters []OfferedApplicationFilter `json:"filters,omitempty"`
}

// OfferedApplicationFilter has a set of filter terms that
// are used by a vendor to query remote applications that the vendor has offered.
type OfferedApplicationFilter struct {
	FilterTerms []OfferedApplicationFilterTerm `json:"filter-terms,omitempty"`
}

// OfferedApplicationFilterTerm has filter criteria that
// are used by a vendor to query remote applications that the vendor has offered.
type OfferedApplicationFilterTerm struct {
	// ApplicationURL is url for remote application.
	// This may be a part of valid URL.
	ApplicationURL string `json:"application-url,omitempty"`

	// Endpoint contains endpoint properties for filter.
	Endpoint RemoteEndpoint `json:"endpoint"`

	// CharmName is the charm name of this application.
	CharmName string `json:"charm-name,omitempty"`
}