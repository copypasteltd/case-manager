import { useMemo, useState } from 'react';
import { MemoryDrafts, type DraftFields, type DraftTarget } from './drafts';
import type { Schemas } from './session';
import { client } from './taro';

const drafts = new MemoryDrafts(() => ({ authenticated: client.authenticated, epoch: client.epoch }));
client.subscribe(() => { if (!client.authenticated) drafts.clear(); });

export function useFormDraft(me: Schemas['Me'], target: DraftTarget, defaultAction: string, defaults: Partial<DraftFields> = {}) {
  const identity = JSON.stringify([client.epoch, me.user.id, me.authorization_version, target]);
  const handle = useMemo(() => drafts.open({ epoch: client.epoch, userID: me.user.id, authorizationVersion: me.authorization_version }, target), [identity]);
  const initial = useMemo(() => handle?.read(), [handle]);
  const [state, setState] = useState<{ handle: typeof handle; fields: DraftFields; retained: boolean }>();
  const fields = state && state.handle === handle ? state.fields : initial ?? { ...defaults, reason: '', action: defaultAction };
  return {
    fields, restored: Boolean(initial), binding: identity, retained: state && state.handle === handle ? state.retained : Boolean(handle),
    update(change: Partial<DraftFields>) { const next = { ...fields, ...change }; const retained = handle?.write(next) ?? false; setState({ handle, fields: next, retained }); },
    forget() { handle?.forget(); },
  };
}
