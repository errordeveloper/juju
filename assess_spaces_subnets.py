#!/usr/bin/env python
from __future__ import print_function
import logging
import re
import yaml
import socket
from textwrap import dedent
from argparse import ArgumentParser

from jujuconfig import (
    get_juju_home,
)
from jujupy import (
    parse_new_state_server_from_error,
    temp_bootstrap_env,
)
from utility import (
    print_now,
    add_basic_testing_arguments,
)
from deploy_stack import (
    get_machine_dns_name,
    dump_env_logs
)
from assess_container_networking import (
    clean_environment,
    ssh,
    get_client,
)

__metaclass__ = type


def parse_args(argv=None):
    """Parse all arguments."""

    description = dedent("""\
    Test container address allocation.
    For LXC and KVM, create machines of each type and test the network
    between LXC <--> LXC, KVM <--> KVM and LXC <--> KVM. Also test machine
    to outside world, DNS and that these tests still pass after a reboot. In
    case of failure pull logs and configuration files from the machine that
    we detected a problem on for later analysis.
    """)
    parser = add_basic_testing_arguments(ArgumentParser(
        description=description
    ))
    parser.add_argument(
        '--clean-environment', action='store_true', help=dedent("""\
        Attempts to re-use an existing environment rather than destroying it
        and creating a new one.

        On launch, if an environment exists, clean out services and machines
        from it rather than destroying it. If an environment doesn't exist,
        create one and use it.

        At termination, clean out services and machines from the environment
        rather than destroying it."""))
    return parser.parse_args(argv)


def assess_spaces_subnets(client):
    """Check that space and subnet functionality works as expected
    :param client: EnvJujuClient
    """
    network_config = {
        'default': ['subnet-0fb97566', 'subnet-d27d91a9'],
        'dmz': ['subnet-604dcd09', 'subnet-882d8cf3'],
        'apps': ['subnet-c13fbfa8', 'subnet-53da7a28'],
        'backend': ['subnet-5e4dcd37', 'subnet-7c2c8d07'],
    }

    charms_to_space = {
        'haproxy': {'space': 'dmz'},
        'mediawiki': {'space': 'apps'},
        'memcached': {'space': 'apps'},
        'mysql': {'space': 'backend'},
        'mysql-slave': {
            'space': 'backend',
            'charm': 'mysql',
        },
    }

    _assess_spaces_subnets(client, network_config, charms_to_space)


def _assess_spaces_subnets(client, network_config, charms_to_space):
    """Check that space and subnet functionality works as expected
    :param client: EnvJujuClient
    :param network_config: Map of 'space name' to ['subnet', 'list']
    :param charms_to_space: Map of 'unit name' to
           {'space': 'space name', 'charm': 'charm name (if not same as unit)}
    :return: None. Raises exception on failure.
    """
    for space in sorted(network_config.keys()):
        client.juju('space create', space)
        for subnet in network_config[space]:
            client.juju('subnet add', (subnet, space))

    for name in sorted(charms_to_space.keys()):
        if 'charm' not in charms_to_space[name]:
            charms_to_space[name]['charm'] = name
        charm = charms_to_space[name]['charm']
        space = charms_to_space[name]['space']
        client.juju('deploy',
                    (charm, name, '--constraints', 'spaces=' + space))

    # Scale up. We don't specify constraints, but they should still be honored
    # per charm.
    client.juju('add-unit', 'mysql-slave')
    client.juju('add-unit', 'mediawiki')
    status = client.wait_for_started()

    spaces = yaml.load(client.get_juju_output('space list'))

    unit_priv_address = {}
    units_found = 0
    for service in sorted(status.status['services'].values()):
        for unit_name, unit in service.get('units', {}).items():
            units_found += 1
            addrs = ssh(client, unit['machine'], 'ip -o addr')
            for addr in re.findall(r'^\d+:\s+(\w+)\s+inet\s+(\S+)',
                                   addrs, re.MULTILINE):
                if addr[0] != 'lo':
                    unit_priv_address[unit_name] = addr[1]

    cidrs_in_space = {}
    for name, attrs in spaces['spaces'].iteritems():
        cidrs_in_space[name] = []
        for cidr in attrs:
            cidrs_in_space[name].append(cidr)

    units_checked = 0
    for space, cidrs in cidrs_in_space.iteritems():
        for cidr in cidrs:
            for unit, address in unit_priv_address.iteritems():
                if ipv4_in_cidr(address, cidr):
                    units_checked += 1
                    charm = unit.split('/')[0]
                    if charms_to_space[charm]['space'] != space:
                        raise ValueError("Found {} in {}, expected {}".format(
                            unit, space, charms_to_space[charm]['space']))

    if units_found != units_checked:
        raise ValueError("Could not find spaces for all units")

    return units_checked


def ipv4_to_int(ipv4):
    """Convert an IPv4 dotted decimal address to an integer"""
    b = [int(b) for b in ipv4.split('.')]
    return b[0] << 24 | b[1] << 16 | b[2] << 8 | b[3]


def ipv4_in_cidr(ipv4, cidr):
    """Returns True if the given address is in the given CIDR"""
    if '/' in ipv4:
        ipv4, _ = ipv4.split('/')
    ipv4 = ipv4_to_int(ipv4)
    value, bits = cidr.split('/')
    subnet = ipv4_to_int(value)
    mask = 0xFFFFFFFF & (0xFFFFFFFF << (32-int(bits)))
    return (ipv4 & mask) == subnet


def main():
    args = parse_args()
    client = get_client(args)
    juju_home = get_juju_home()
    bootstrap_host = None
    try:
        if args.clean_environment:
            try:
                if not clean_environment(client):
                    with temp_bootstrap_env(juju_home, client):
                        client.bootstrap(args.upload_tools)
            except Exception as e:
                logging.exception(e)
                client.destroy_environment()
                client = get_client(args)
                with temp_bootstrap_env(juju_home, client):
                    client.bootstrap(args.upload_tools)
        else:
            client.destroy_environment()
            client = get_client(args)
            with temp_bootstrap_env(juju_home, client):
                client.bootstrap(args.upload_tools)

        logging.info('Waiting for the bootstrap machine agent to start.')
        client.wait_for_started()
        bootstrap_host = get_machine_dns_name(client, 0)

        assess_spaces_subnets(client)

    except Exception as e:
        logging.exception(e)
        try:
            if bootstrap_host is None:
                bootstrap_host = parse_new_state_server_from_error(e)
        except Exception as e:
            print_now('exception while dumping logs:\n')
            logging.exception(e)
        exit(1)
    finally:
        if bootstrap_host is not None:
            dump_env_logs(client, bootstrap_host, args.logs)
        if not args.keep_env:
            if args.clean_environment:
                clean_environment(client)
            else:
                client.destroy_environment()


if __name__ == '__main__':
    main()
