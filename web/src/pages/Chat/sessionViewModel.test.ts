import { describe, expect, it } from 'vitest';
import {
  buildChatSessionHref,
  getSessionDisplayName,
  getSessionSubtitle,
  sortSessionsByRecent,
  type SessionWithProject,
} from './sessionViewModel';

function makeSession(overrides: Partial<SessionWithProject> = {}): SessionWithProject {
  return {
    id: 'sess-1',
    session_key: 'bridge:default',
    name: '',
    platform: 'feishu',
    agent_type: 'codex',
    active: true,
    live: true,
    created_at: '2026-05-13T01:00:00Z',
    updated_at: '2026-05-13T02:00:00Z',
    history_count: 3,
    last_message: null,
    project: 'alpha',
    ...overrides,
  };
}

describe('sessionViewModel', () => {
  it('prefers participant name over generic session identifiers', () => {
    const session = makeSession({
      name: 'fallback-session',
      user_name: 'Alice',
      chat_name: 'Team Chat',
    });

    expect(getSessionDisplayName(session)).toBe('Alice');
  });

  it('builds chat links to a specific session route', () => {
    expect(buildChatSessionHref('alpha', 'sess-9')).toBe('/chat/alpha/sess-9');
  });

  it('shows chat name as secondary context when both participant and chat exist', () => {
    const session = makeSession({
      project: 'alpha',
      platform: 'feishu',
      user_name: 'Alice',
      chat_name: 'Release Room',
    });

    expect(getSessionSubtitle(session)).toBe('Release Room / alpha / feishu');
  });

  it('sorts sessions by most recently updated first', () => {
    const newer = makeSession({ id: 'newer', updated_at: '2026-05-13T03:00:00Z' });
    const older = makeSession({ id: 'older', updated_at: '2026-05-13T01:30:00Z' });

    expect(sortSessionsByRecent([older, newer]).map((s) => s.id)).toEqual(['newer', 'older']);
  });
});
