// Transient command reconciliation, separate from editable input drafts.
// Exact request signatures stay in bounded RAM only; they are never restored
// into forms, logged, persisted or automatically sent. Auth/MFA is excluded.
type Entry = { key: string; signature: string; state: 'pending' | 'uncertain' | 'succeeded' };
export class UnresolvedCommand extends Error {}
export class CommandKeys {
  private entries = new Map<string, Entry>();
  constructor(private create: () => string, private maxCharacters = 4 * 1048576, private maxEntries = 128) {}
  clear() { this.entries.clear(); }
  reserve(binding: string, signature: string) {
    const previous = this.entries.get(binding);
    if (previous?.signature === signature) return previous.key;
    if (previous && previous.state !== 'succeeded') throw new UnresolvedCommand('上次操作的结果尚未核对。请先查看当前资料，再使用原输入核对本次结果；不能直接改写为另一次创建。');
    const characters = [...this.entries.entries()].reduce((sum, [name, entry]) => sum + name.length + entry.signature.length, 0);
    const replaced = previous ? binding.length + previous.signature.length : 0;
    if (!binding || !signature || characters - replaced + binding.length + signature.length > this.maxCharacters || (!previous && this.entries.size >= this.maxEntries)) throw new UnresolvedCommand('尚有未核对的操作占用本次会话容量。请先核对这些结果，再开始新操作。');
    const key = this.create(); this.entries.set(binding, { key, signature, state: 'pending' }); return key;
  }
  settle(binding: string, key: string, state: 'uncertain' | 'succeeded' | 'rejected', observed = false) {
    const entry = this.entries.get(binding);
    if (!entry || entry.key !== key) return;
    if (state === 'rejected' || (state === 'succeeded' && observed)) this.entries.delete(binding);
    else entry.state = state;
  }
}
