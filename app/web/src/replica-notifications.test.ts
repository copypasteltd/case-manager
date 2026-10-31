import { describe, expect, it } from 'vitest';
import { notificationDestination, replicaVersionSelection, checkedReplicaVersion } from '@copypaste/contracts/notifications';
import type { components } from '@copypaste/contracts';
type Version = components['schemas']['FileVersion'];
const id = (n: number) => '10000000-0000-4000-8000-' + String(n).padStart(12, '0');
const target = () => ({ type: 'file' as const, id: id(1), container_id: null, web_path: '/files/' + id(1), version_id: id(2), replica_id: id(3) });
const version = () => ({ id: id(2), file_id: id(1), replicas: [{ id: id(3) }] } as Version);

describe('exact replica notification destinations', () => {
  it('opens the exact retained version and replica on Web and shared mobile', () => {
    expect(notificationDestination(target())).toEqual({ web: '/files/' + id(1) + '?version=' + id(2) + '&replica=' + id(3), mobile: '/pages/detail/index?kind=file&id=' + id(1) + '&version=' + id(2) + '&replica=' + id(3) });
  });
  it('keeps safe legacy file links and rejects unpaired or unrelated replica fields', () => {
    const { version_id, replica_id, ...legacy } = target();
    expect(notificationDestination(legacy).web).toBe('/files/' + id(1));
    for (const value of [{ ...legacy, version_id }, { ...legacy, replica_id }, { ...target(), type: 'request' as const }, { ...target(), replica_id: '../secrets' }, { ...target(), version_id: id(2) + '&run=1' }]) expect(() => notificationDestination(value)).toThrow();
  });
  it('never accepts a returned redirect or a different resource path', () => {
    for (const path of ['https://example.org', '/files/' + id(9), '/files/' + id(1) + '?version=' + id(9), '//example.org']) expect(() => notificationDestination({ ...target(), web_path: path })).toThrow();
  });
  it('loads general fixed versions while requiring a version for every replica', () => {
    expect(replicaVersionSelection(null, undefined)).toBeNull();
    expect(replicaVersionSelection(id(2), null)).toEqual({ versionID: id(2), replicaID: null });
    expect(() => replicaVersionSelection(null, id(3))).toThrow();
    expect(() => replicaVersionSelection('', id(3))).toThrow();
  });
  it('rejects a wrong file/head/version or a missing and duplicated replica in a fetched reply', () => {
    const selection = replicaVersionSelection(id(2), id(3))!;
    expect(checkedReplicaVersion(version(), id(1), selection)).toEqual(version());
    for (const change of [{ id: id(8) }, { file_id: id(8) }, { replicas: [] }, { replicas: [{ id: id(8) }] }, { replicas: [{ id: id(3) }, { id: id(3) }] }]) expect(() => checkedReplicaVersion({ ...version(), ...change } as Version, id(1), selection)).toThrow();
  });
});
