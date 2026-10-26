import { ed25519 } from '@noble/curves/ed25519.js';
import { sha256 } from '@noble/hashes/sha2.js';
import { decodeContentBase64, decodeContentText } from './content';
import { utf8Bytes } from './encoding';
import type { Schemas } from './session';

const uuid = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
function decode(value: string, size?: number) {
  if (typeof value !== 'string' || value.length > 5464) throw new Error('Invalid proof');
  const padding = value.endsWith('==') ? 2 : value.endsWith('=') ? 1 : 0;
  const length = value.length / 4 * 3 - padding;
  if (!Number.isInteger(length) || length < 1 || length > 4096 || size !== undefined && length !== size) throw new Error('Invalid proof');
  return decodeContentBase64(value, length);
}
// Bind the exact selected history row, raw payload and signed source domain.
// Public keys are the historical keys returned by the authenticated API, not
// an independent assertion that a provider or service is currently healthy.
export function verifyStatusProof(proof: Schemas['ServiceObservationProof'], selected: Schemas['ServiceObservation']): boolean {
  try {
    if (!uuid.test(proof.id) || !uuid.test(proof.source_id) || !uuid.test(proof.source_revision_id)) return false;
    for (const field of ['id', 'source_id', 'source_revision_id', 'sequence', 'healthy', 'latency_ms', 'payload_sha256', 'applied', 'ignored_reason'] as const) if (proof[field] !== selected[field]) return false;
    for (const field of ['observed_at', 'received_at'] as const) if (!Number.isFinite(Date.parse(proof[field])) || Date.parse(proof[field]) !== Date.parse(selected[field])) return false;
    if (!/^[a-f0-9]{64}$/.test(proof.payload_sha256) || !/^[1-9]\d*$/.test(proof.sequence) || !Number.isSafeInteger(Number(proof.sequence))) return false;
    const payload = decode(proof.signed_payload), raw = JSON.parse(decodeContentText(payload)) as unknown;
    if (!raw || typeof raw !== 'object' || Array.isArray(raw) || Object.keys(raw).sort().join(',') !== 'healthy,latency_ms,observed_at,sequence,source_revision_id') return false;
    const value = raw as Record<string, unknown>;
    if (value.source_revision_id !== proof.source_revision_id || value.sequence !== proof.sequence || value.healthy !== proof.healthy || value.latency_ms !== proof.latency_ms || typeof value.observed_at !== 'string' || Date.parse(value.observed_at) !== Date.parse(proof.observed_at)) return false;
    const digest = Array.from(sha256(payload), byte => byte.toString(16).padStart(2, '0')).join('');
    if (digest !== proof.payload_sha256) return false;
    const prefix = utf8Bytes('CP-STATUS-V1\n' + proof.source_id + '\n'), message = new Uint8Array(prefix.length + payload.length);
    message.set(prefix); message.set(payload, prefix.length);
    return ed25519.verify(decode(proof.signature, 64), message, decode(proof.public_key, 32), { zip215: false });
  } catch { return false; }
}
