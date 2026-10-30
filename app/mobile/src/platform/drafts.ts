import { copySubscriptionInput, subscriptionInputCharacters, type SubscriptionInput } from './subscription-input';
import { copyFinancialInput, financialInputCharacters, type FinancialInput } from './financial-input';
import { copyChangeInput, changeInputCharacters, type ChangeInput } from './change-input';
import { copyProjectInput, projectInputCharacters, type ProjectInput } from './project-input';
import { copyResourceInput, resourceInputCharacters, type ResourceInput } from './project-resource-input';
import { copyTaskInput, taskInputCharacters, type TaskInput } from './task-input';
import { businessInputCharacters, copyBusinessInput, type BusinessInput } from './business-input';
import { copyIncidentInput, incidentInputCharacters, type IncidentInput } from './incident-input';
// Short-lived, explicitly whitelisted inputs only. Never credentials, fixed
// evidence, MFA, acknowledgement flags, API responses or command payloads.
export type DraftFields = { reason: string; action: string; name?: string; criteria?: string; description?: string; body?: string; owner?: string; milestone?: string; business?: BusinessInput; incident?: IncidentInput; project?: ProjectInput; resource?: ResourceInput; task?: TaskInput; change?: ChangeInput; financial?: FinancialInput; subscription?: SubscriptionInput };
export type DraftAuthority = { epoch: number; userID: string; authorizationVersion: number };
export type DraftTarget = { kind: 'task' | 'task-create' | 'task-edit' | 'task-dependency' | 'deliverable-create' | 'deliverable-edit' | 'deliverable-submit' | 'task-comment-create' | 'task-comment-edit' | 'comment-create' | 'comment-edit'; id: string; version: number } | {
  kind: 'approval'; id: string; version: number; revision: string; hash: string; step: string; stepVersion: number;
} | {
  kind: 'change-create' | 'change-edit'; id: string; version: number; hash: string;
} | {
  kind: 'project-archive'; id: string; version: number; hash: string;
} | {
  kind: 'subscription-edit' | 'subscription-action' | 'subscription-period-action' | 'financial-edit' | 'financial-action' | 'deliverable-decision' | 'change-prepare' | 'change-implement'; id: string; version: number; revision: string; hash: string;
} | {
  kind: 'folder-create' | 'folder-organize' | 'file-organize' | 'file-state' | 'financial-create' | 'subscription-create' | 'project-closeout-transition' | 'resource-create' | 'resource-edit' | 'resource-transition' | 'project-create' | 'project-edit' | 'project-member' | 'project-owner' | 'business-contract' | 'business-purchase' | 'business-claim' | 'request-prepare' | 'request-submit' | 'request-revise' | 'request-withdraw' | 'incident-create' | 'incident-update' | 'conversation-create' | 'conversation-edit' | 'conversation-member' | 'run-create' | 'run-cancel' | 'agent-project-plan' | 'agent-plan-action' | 'agent-organization-action'; id: string; version: number;
};
export interface DraftHandle { read(): DraftFields | undefined; write(fields: DraftFields): boolean; forget(): void }
type Entry = { fields: DraftFields; changed: number };
const TTL = 15 * 60000, MAX_ENTRIES = 32, MAX_CHARACTERS = 100000;

export class MemoryDrafts {
  private authority: DraftAuthority | undefined;
  private entries = new Map<string, Entry>();
  constructor(private session: () => { authenticated: boolean; epoch: number }, private now = () => Date.now()) {}
  clear() { this.authority = undefined; this.entries.clear(); }
  private sweep() {
    const cutoff = this.now() - TTL;
    for (const [key, value] of this.entries) if (value.changed <= cutoff) this.entries.delete(key);
  }
  open(authority: DraftAuthority, target: DraftTarget): DraftHandle | undefined {
    const current = this.session();
    if (!current.authenticated || current.epoch !== authority.epoch || !authority.userID || !Number.isSafeInteger(authority.authorizationVersion) || authority.authorizationVersion < 1) return;
    if (!Number.isSafeInteger(target.version) || target.version < 1 || !target.id || (target.kind === 'approval' && (!target.revision || !target.hash || !target.step || !Number.isSafeInteger(target.stepVersion) || target.stepVersion < 1)) || (target.kind === 'project-archive' && !/^[a-f0-9]{32}$/.test(target.hash)) || (['change-create', 'change-edit'].includes(target.kind) && !/^[a-f0-9]{64}$/.test((target as { hash: string }).hash)) || (('revision' in target && target.kind !== 'approval') && (!target.revision || !/^[a-f0-9]{64}$/.test(target.hash)))) return;
    if (this.authority?.epoch === authority.epoch && (this.authority.userID !== authority.userID || this.authority.authorizationVersion > authority.authorizationVersion)) return;
    if (!this.authority || this.authority.epoch !== authority.epoch || this.authority.authorizationVersion !== authority.authorizationVersion) {
      this.clear(); this.authority = { ...authority };
    }
    const bound = this.authority, epoch = authority.epoch;
    // Fixed field order prevents accidental equivalence between revisions.
    const key = JSON.stringify(target.kind === 'approval' ? ['approval', target.id, target.version, target.revision, target.hash, target.step, target.stepVersion] : 'revision' in target ? [target.kind, target.id, target.version, target.revision, target.hash] : 'hash' in target ? [target.kind, target.id, target.version, target.hash] : [target.kind, target.id, target.version]);
    const valid = () => { const state = this.session(); return state.authenticated && state.epoch === epoch && this.authority === bound; };
    return {
      read: () => { if (!valid()) return; this.sweep(); const value = this.entries.get(key); return value ? { ...value.fields, ...(value.fields.subscription ? { subscription: copySubscriptionInput(value.fields.subscription) } : {}), ...(value.fields.business ? { business: copyBusinessInput(value.fields.business) } : {}), ...(value.fields.incident ? { incident: copyIncidentInput(value.fields.incident) } : {}), ...(value.fields.project ? { project: copyProjectInput(value.fields.project) } : {}), ...(value.fields.resource ? { resource: copyResourceInput(value.fields.resource) } : {}), ...(value.fields.task ? { task: copyTaskInput(value.fields.task) } : {}), ...(value.fields.financial ? { financial: copyFinancialInput(value.fields.financial) } : {}), ...(value.fields.change ? { change: copyChangeInput(value.fields.change) } : {}) } : undefined; },
      write: fields => {
        if (!valid() || typeof fields.reason !== 'string' || typeof fields.action !== 'string') return false;
        const limits = { reason: 20000, action: 40, name: 200, criteria: 20000, description: 20000, body: 20000, owner: 36, milestone: 36 };
        const copy: Partial<DraftFields> = {};
        for (const name of Object.keys(limits) as (keyof typeof limits)[]) {
          const value = fields[name];
          if (value === undefined && name !== 'reason' && name !== 'action') continue;
          if (typeof value !== 'string' || value.length > (name === 'name' && ['folder-create', 'folder-organize', 'file-organize'].includes(target.kind) ? 400 : limits[name])) return false;
          copy[name] = value;
        }
        if (fields.subscription !== undefined) { const input = copySubscriptionInput(fields.subscription); if (!input) return false; copy.subscription = input; }
        if (fields.business !== undefined) {
          const business = copyBusinessInput(fields.business);
          if (!business) return false;
          copy.business = business;
        }
        if (fields.incident !== undefined) {
          const incident = copyIncidentInput(fields.incident);
          if (!incident) return false;
          copy.incident = incident;
        }
        if (fields.project !== undefined) {
          const project = copyProjectInput(fields.project);
          if (!project) return false;
          copy.project = project;
        }
        if (fields.resource !== undefined) {
          const resource = copyResourceInput(fields.resource);
          if (!resource) return false;
          copy.resource = resource;
        }
        if (fields.task !== undefined) {
          const task = copyTaskInput(fields.task);
          if (!task) return false;
          copy.task = task;
        }
        if (fields.change !== undefined) {
          const change = copyChangeInput(fields.change);
          if (!change) return false;
          copy.change = change;
        }
        if (fields.financial !== undefined) {
          const financial = copyFinancialInput(fields.financial);
          if (!financial) return false;
          copy.financial = financial;
        }
        this.sweep(); this.entries.delete(key);
        this.entries.set(key, { fields: copy as DraftFields, changed: this.now() });
        const length = (value: DraftFields) => Object.entries(value).reduce((sum, [key, text]) => sum + (typeof text === 'string' ? text.length : key === 'subscription' ? subscriptionInputCharacters(text as SubscriptionInput) : key === 'financial' ? financialInputCharacters(text as FinancialInput) : key === 'incident' ? incidentInputCharacters(text as IncidentInput) : key === 'project' ? projectInputCharacters(text as ProjectInput) : key === 'resource' ? resourceInputCharacters(text as ResourceInput) : key === 'task' ? taskInputCharacters(text as TaskInput) : key === 'change' ? changeInputCharacters(text as ChangeInput) : businessInputCharacters(text as BusinessInput)), 0);
        let characters = [...this.entries.values()].reduce((sum, entry) => sum + length(entry.fields), 0);
        for (const [oldest, entry] of this.entries) {
          if (this.entries.size <= MAX_ENTRIES && characters <= MAX_CHARACTERS) break;
          characters -= length(entry.fields); this.entries.delete(oldest);
        }
        return this.entries.has(key);
      },
      forget: () => { if (valid()) this.entries.delete(key); },
    };
  }
}
