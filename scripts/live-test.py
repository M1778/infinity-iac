import argparse
import datetime
import fcntl
import ipaddress
import json
import os
from pathlib import Path
import re
import shlex
import signal
import subprocess
import sys
import time
import urllib.request


def atomic_json(path, data):
    temporary = path.with_suffix('.tmp')
    descriptor = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(descriptor, 'w') as stream:
        json.dump(data, stream, indent=2)
        stream.flush()
        os.fsync(stream.fileno())
    temporary.replace(path)


class LiveRun:
    def __init__(self, args):
        import oci
        self.oci = oci
        self.args = args
        self.work = Path(args.work).resolve()
        self.work.mkdir(mode=0o700, parents=True, exist_ok=True)
        if self.work.is_symlink():
            raise RuntimeError('test workspace must not be a symlink')
        self.journal = self.work / 'cloud-resources.json'
        self.config = oci.config.from_file(str(Path(args.config).expanduser()), args.profile)
        self.compartment = args.compartment or self.config['tenancy']
        options = dict(timeout=(10, 30), retry_strategy=oci.retry.NoneRetryStrategy())
        self.compute = oci.core.ComputeClient(self.config, **options)
        self.network = oci.core.VirtualNetworkClient(self.config, **options)
        self.block = oci.core.BlockstorageClient(self.config, **options)
        if self.journal.exists():
            self.state = json.loads(self.journal.read_text())
            if self.state['compartment'] != self.compartment:
                raise RuntimeError('test journal belongs to another compartment')
        else:
            if args.cleanup or args.cleanup_check:
                raise RuntimeError('cleanup requires a recorded test journal')
            self.state = {'schema': 1, 'run': self.work.name, 'access': 'live-' + self.work.name,
                          'compartment': self.compartment, 'region': self.config['region'],
                          'name': 'iac-' + self.work.name, 'resources': {}, 'instances': [],
                          'boot_volumes': [], 'tests': [], 'status': 'prepared'}
            self.save()
        self.deadline = time.monotonic() + args.deadline
        self.env = dict(os.environ, INFINITY_IAC_CONFIG=str(self.work / 'access.json'),
                        OCI_CONFIG_FILE=str(Path(args.config).expanduser()))

    def save(self):
        atomic_json(self.journal, self.state)

    def log(self, text):
        print(datetime.datetime.now(datetime.timezone.utc).isoformat(timespec='seconds') + ' ' + text, flush=True)

    def check(self, name, detail):
        self.state['tests'].append({'name': name, 'passed': True, 'detail': detail})
        self.save()
        self.log('PASS ' + name + ': ' + detail)

    def command(self, args, seconds=120, input_text=None):
        if time.monotonic() >= self.deadline and not self.args.cleanup:
            raise TimeoutError('test deadline reached')
        result = subprocess.run([str(Path(self.args.binary).resolve())] + args, env=self.env,
                                capture_output=True, text=True, timeout=seconds, input=input_text)
        path = self.work / ('command-' + str(time.time_ns()) + '.log')
        path.write_text(result.stdout + result.stderr)
        path.chmod(0o600)
        if result.returncode:
            raise RuntimeError('CLI ' + ' '.join(args[:2]) + ' failed; private log ' + path.name)
        return result.stdout

    def inventory(self, fn, **kwargs):
        return self.oci.pagination.list_call_get_all_results(fn, **kwargs).data

    def get(self, fn, identifier):
        try:
            return fn(identifier).data
        except self.oci.exceptions.ServiceError as error:
            if error.status == 404:
                return None
            raise

    def own(self, item):
        if item.compartment_id != self.compartment or item.freeform_tags.get('infinity-iac') != 'true' or item.freeform_tags.get('infinity-iac-access') != self.state['access']:
            raise RuntimeError('refusing a resource outside the exact test access profile')

    def wait(self, getter, identifier, state, seconds=600):
        deadline = time.monotonic() + seconds
        while time.monotonic() < deadline:
            item = self.get(getter, identifier)
            if item is None and state == 'TERMINATED':
                return None
            if item is None:
                raise RuntimeError('resource disappeared before reaching ' + state)
            if item.lifecycle_state == state:
                return item
            if item.lifecycle_state in ('FAILED', 'FAULTY'):
                raise RuntimeError('resource reached ' + item.lifecycle_state)
            time.sleep(5)
        raise TimeoutError('resource did not reach ' + state)

    def discover_owned(self):
        instances = self.inventory(self.compute.list_instances, compartment_id=self.compartment)
        for item in instances:
            if item.freeform_tags.get('infinity-iac-access') == self.state['access']:
                self.own(item)
                if item.id not in self.state['instances']:
                    self.state['instances'].append(item.id)
                attachments = self.inventory(self.compute.list_boot_volume_attachments,
                                             availability_domain=item.availability_domain,
                                             compartment_id=self.compartment, instance_id=item.id)
                for attachment in attachments:
                    if attachment.instance_id != item.id:
                        raise RuntimeError('boot volume attachment belongs to another instance')
                    if attachment.boot_volume_id not in self.state['boot_volumes']:
                        self.state['boot_volumes'].append(attachment.boot_volume_id)
        vcns = self.inventory(self.network.list_vcns, compartment_id=self.compartment)
        for item in vcns:
            if item.freeform_tags.get('infinity-iac-access') == self.state['access']:
                self.own(item)
                if self.state['resources'].get('vcn') not in (None, item.id):
                    raise RuntimeError('more than one VCN belongs to the test access')
                self.state['resources'].update(vcn=item.id, route_table=item.default_route_table_id,
                                               security_list=item.default_security_list_id,
                                               dhcp_options=item.default_dhcp_options_id)
        self.save()

    def cleanup(self):
        self.args.cleanup = True
        self.discover_owned()
        self.state['status'] = 'cleaning'
        self.save()
        for identifier in self.state['instances']:
            item = self.get(self.compute.get_instance, identifier)
            if item is None or item.lifecycle_state == 'TERMINATED':
                continue
            self.own(item)
            if item.lifecycle_state != 'TERMINATING':
                self.compute.terminate_instance(identifier, preserve_boot_volume=False)
            self.wait(self.compute.get_instance, identifier, 'TERMINATED')
        for identifier in self.state['boot_volumes']:
            self.wait(self.block.get_boot_volume, identifier, 'TERMINATED')
        identifier = self.state['resources'].get('vcn')
        vcn = self.get(self.network.get_vcn, identifier) if identifier else None
        if vcn is not None:
            self.own(vcn)
            subnets = self.inventory(self.network.list_subnets, compartment_id=self.compartment, vcn_id=vcn.id)
            gateways = self.inventory(self.network.list_internet_gateways, compartment_id=self.compartment, vcn_id=vcn.id)
            for subnet in subnets:
                self.own(subnet)
                if subnet.vcn_id != vcn.id:
                    raise RuntimeError('subnet is not associated with the exact test VCN')
                if self.inventory(self.network.list_private_ips, subnet_id=subnet.id):
                    raise RuntimeError('test subnet still has VNIC dependencies; refusing network cleanup')
                self.network.delete_subnet(subnet.id)
                self.wait(self.network.get_subnet, subnet.id, 'TERMINATED')
            route = self.get(self.network.get_route_table, vcn.default_route_table_id)
            if route is not None:
                if route.vcn_id != vcn.id:
                    raise RuntimeError('route table belongs to another VCN')
                self.network.update_route_table(route.id, self.oci.core.models.UpdateRouteTableDetails(route_rules=[]))
            for gateway in gateways:
                self.own(gateway)
                if gateway.vcn_id != vcn.id:
                    raise RuntimeError('gateway belongs to another VCN')
                self.network.delete_internet_gateway(gateway.id)
                self.wait(self.network.get_internet_gateway, gateway.id, 'TERMINATED')
            self.network.delete_vcn(vcn.id)
            self.wait(self.network.get_vcn, vcn.id, 'TERMINATED')
        self.state['status'] = 'cleanup-verified'
        self.state['cleaned_at'] = datetime.datetime.now(datetime.timezone.utc).isoformat()
        self.save()
        self.log('PASS cleanup: exact test instances, boot volumes, and network removed')

    def ssh(self, address, command, seconds=60, input_text=None):
        ipaddress.ip_address(address)
        result = subprocess.run(['ssh', '-i', str(self.work / 'ssh_key'), '-o', 'BatchMode=yes',
                                 '-o', 'StrictHostKeyChecking=accept-new', '-o', 'UserKnownHostsFile=' + str(self.work / 'known_hosts'),
                                 '-o', 'ConnectTimeout=10', 'ubuntu@' + address, command],
                                capture_output=True, text=True, input=input_text, timeout=seconds)
        if result.returncode:
            log = self.work / ('ssh-' + str(time.time_ns()) + '.log')
            log.write_text(result.stdout + result.stderr)
            log.chmod(0o600)
            raise RuntimeError('SSH command failed; private log ' + log.name)
        return result.stdout

    def assert_preserved(self):
        for identifier, before in self.state.get('preserved_instances', {}).items():
            current = self.get(self.compute.get_instance, identifier)
            if current is None or current.lifecycle_state != before:
                raise RuntimeError('pre-existing instance lifecycle changed during the test')
        for identifier in self.state.get('preserved_vcns', []):
            if self.get(self.network.get_vcn, identifier) is None:
                raise RuntimeError('pre-existing VCN is missing')
        self.check('personal-resources', 'pre-existing instance states and VCNs preserved')

    def run(self):
        if self.state['status'] != 'prepared':
            raise RuntimeError('a live test requires a fresh prepared workspace')
        if not self.args.image or not self.args.availability_domain:
            raise RuntimeError('explicit image and availability domain required')
        self.state['preserved_instances'] = {item.id: item.lifecycle_state for item in self.inventory(self.compute.list_instances, compartment_id=self.compartment) if item.lifecycle_state != 'TERMINATED'}
        self.state['preserved_vcns'] = [item.id for item in self.inventory(self.network.list_vcns, compartment_id=self.compartment)]
        self.save()
        key = self.work / 'ssh_key'
        subprocess.run(['ssh-keygen', '-q', '-t', 'ed25519', '-N', '', '-f', str(key)], check=True, timeout=30)
        self.command(['setup', 'oracle', '--autodetect', '--network-only', '--name', self.state['access'], '--profile', self.args.profile, '--config', self.args.config, '--compartment', self.compartment])
        self.check('network-profile', 'read-only setup validates local config and compartment')
        public_ip = ipaddress.ip_address(self.args.ssh_source.removesuffix('/32'))
        if public_ip.version != 4:
            raise RuntimeError('test SSH source requires an IPv4 /32')
        network = json.loads(self.command(['network', 'create', '--name', self.state['name'], '--access', self.state['access'], '--cidr', '10.89.0.0/16', '--subnet-cidr', '10.89.42.0/24', '--ssh-source', str(public_ip) + '/32', '--yes', '--output', 'json'], 660))
        self.state['resources'].update(vcn=network['id'], subnet=network['subnet_id'], gateway=network['internet_gateway_id'], route_table=network['route_table_id'], security_list=network['security_list_id'])
        self.save()
        self.command(['network', 'get', network['id'], '--access', self.state['access'], '--output', 'json'])
        self.command(['network', 'update', network['id'], '--access', self.state['access'], '--ssh-source', str(public_ip) + '/32', '--yes', '--output', 'json'])
        self.check('network-crud', 'create, owned inspection, and ETag-guarded SSH update passed')
        self.command(['setup', 'oracle', '--autodetect', '--name', self.state['access'], '--profile', self.args.profile, '--config', self.args.config, '--compartment', self.compartment, '--subnet', network['subnet_id'], '--availability-domain', self.args.availability_domain, '--image', self.args.image, '--shape', 'VM.Standard.A1.Flex', '--ssh-public-key', key.with_suffix('.pub').read_text().strip()])
        instance_name = self.state['name'] + '-server'
        bootstrap = self.command(['recipe', 'render', '--name', 'nginx', '--port', '28080'])
        startup = self.work / 'bootstrap.sh'
        startup.write_text(bootstrap)
        startup.chmod(0o600)
        try:
            instances = json.loads(self.command(['create', 'server', '--name', instance_name, '--access', self.state['access'], '--cpu', '2', '--ram', '12GB', '--storage-size', '50GB', '--startup-script', str(startup), '--yes', '--output', 'json'], 210))
        finally:
            self.discover_owned()
        if len(instances) != 1:
            raise RuntimeError('expected exactly one launched test instance')
        identifier = instances[0]['id']
        self.wait(self.compute.get_instance, identifier, 'RUNNING')
        self.discover_owned()
        address = None
        deadline = time.monotonic() + 300
        while time.monotonic() < deadline:
            inspected = json.loads(self.command(['instance', 'get', identifier, '--access', self.state['access'], '--output', 'json']))
            address = inspected.get('public_ip')
            if address:
                try:
                    self.ssh(address, 'true')
                    break
                except RuntimeError:
                    pass
            time.sleep(10)
        else:
            raise TimeoutError('SSH did not become available; check exact test /32 source')
        self.ssh(address, 'sudo timeout 1500 cloud-init status --wait', 1530)
        self.ssh(address, 'sudo test -s /var/lib/infinity-iac/ready/nginx && curl -fsS http://127.0.0.1:28080/ | grep -q "Welcome to nginx"')
        self.check('cloud-init-nginx', 'actual rendered ARM bootstrap, success marker, and HTTP response passed')
        for recipe in self.args.recipes.split(','):
            if not re.fullmatch(r'[a-z][a-z0-9-]*', recipe):
                raise RuntimeError('invalid runtime recipe selector')
            if recipe == 'nginx':
                continue
            script = self.command(['recipe', 'render', '--name', recipe, '--port', '28081'])
            self.ssh(address, 'sudo timeout 2400 bash -s', 2430, script)
            self.ssh(address, 'sudo test -s ' + shlex.quote('/var/lib/infinity-iac/ready/' + recipe))
            self.check('recipe-' + recipe, 'rendered guest commands and bounded protocol readiness passed on ARM')
        self.command(['list', 'instances', '--access', self.state['access'], '--output', 'json'])
        self.command(['instance', 'stop', identifier, '--access', self.state['access'], '--yes', '--wait', '--timeout', '10m'], 660)
        self.command(['instance', 'restart', identifier, '--access', self.state['access'], '--yes', '--wait', '--timeout', '10m'], 660)
        self.check('stop-restart', 'STOPPED to RUNNING through restart START behavior passed')
        self.command(['instance', 'restart', identifier, '--access', self.state['access'], '--yes', '--wait', '--timeout', '10m'], 660)
        self.check('running-restart', 'running instance soft-reset accepted and running state confirmed')
        self.command(['instance', 'delete', identifier, '--access', self.state['access'], '--yes', '--wait', '--timeout', '10m'], 660)
        self.check('compute-delete', 'termination and recorded boot volume cleanup confirmed through CLI')
        self.command(['network', 'delete', network['id'], '--access', self.state['access'], '--yes', '--wait', '--timeout', '10m'], 660)
        self.check('network-delete', 'dependency-ordered deletion confirmed through CLI')
        self.cleanup()
        self.assert_preserved()


def main():
    parser = argparse.ArgumentParser(description='Explicit opt-in CLI/OCI lifecycle test; all created resources are removed.')
    parser.add_argument('--binary', required=True)
    parser.add_argument('--work', required=True)
    parser.add_argument('--config', default=str(Path.home() / '.oci/config'))
    parser.add_argument('--profile', default='DEFAULT')
    parser.add_argument('--compartment')
    parser.add_argument('--image')
    parser.add_argument('--availability-domain')
    parser.add_argument('--ssh-source')
    parser.add_argument('--recipes', default='redis,postgres,wordpress')
    parser.add_argument('--deadline', type=int, default=7200)
    parser.add_argument('--yes', action='store_true')
    parser.add_argument('--prepare', action='store_true')
    parser.add_argument('--cleanup', action='store_true')
    parser.add_argument('--cleanup-check', action='store_true')
    args = parser.parse_args()
    if not args.prepare and not args.cleanup and not args.cleanup_check and (not args.yes or os.getenv('INFINITY_IAC_LIVE_TEST') != '1'):
        parser.error('live allocation requires --yes and INFINITY_IAC_LIVE_TEST=1; establish independent cleanup before running')
    if args.deadline < 60 or args.deadline > 14400:
        parser.error('deadline must be between 60 and 14400 seconds')
    run = LiveRun(args)
    if args.prepare:
        run.log('Prepared private journal; no cloud resources created')
        return
    if args.cleanup_check:
        run.discover_owned()
        if run.state['instances'] or run.state['resources']:
            raise RuntimeError('cleanup preflight requires an empty fresh journal')
        run.state['cleanup_preflight'] = True
        run.save()
        run.log('PASS independent cleanup preflight: credentials work and no test resources exist')
        return
    if args.cleanup:
        run.cleanup()
        run.assert_preserved()
        return
    if not run.state.get('cleanup_preflight'):
        raise RuntimeError('run --cleanup-check through the independent cleanup path before allocation')
    def interrupted(signum, frame):
        raise InterruptedError('live test interrupted')
    signal.signal(signal.SIGTERM, interrupted)
    signal.signal(signal.SIGINT, interrupted)
    signal.signal(signal.SIGALRM, interrupted)
    signal.alarm(args.deadline)
    try:
        run.run()
    except BaseException as error:
        run.state['failure'] = type(error).__name__ + ': ' + str(error)
        run.save()
        raise
    finally:
        signal.alarm(0)
        if run.state.get('status') != 'cleanup-verified':
            run.cleanup()
            run.assert_preserved()


if __name__ == '__main__':
    main()
