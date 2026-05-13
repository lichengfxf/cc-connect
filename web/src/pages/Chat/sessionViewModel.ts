import type { Session } from '@/api/sessions';

export interface SessionWithProject extends Session {
  project: string;
}

function sessionTimestamp(session: Pick<Session, 'updated_at' | 'created_at'>): number {
  return new Date(session.updated_at || session.created_at || 0).getTime();
}

export function sortSessionsByRecent<T extends Pick<Session, 'updated_at' | 'created_at'>>(sessions: T[]): T[] {
  return [...sessions].sort((a, b) => sessionTimestamp(b) - sessionTimestamp(a));
}

export function getSessionDisplayName(session: Pick<Session, 'user_name' | 'chat_name' | 'name' | 'session_key' | 'id'>): string {
  return session.user_name || session.chat_name || session.name || session.session_key || session.id;
}

export function getSessionSubtitle(
  session: Pick<Session, 'user_name' | 'chat_name' | 'platform'> & { project: string },
): string {
  const contextName = session.user_name && session.chat_name ? session.chat_name : '';
  return [contextName, session.project, session.platform].filter(Boolean).join(' / ');
}

export function buildChatSessionHref(project: string, sessionId: string): string {
  return `/chat/${encodeURIComponent(project)}/${encodeURIComponent(sessionId)}`;
}

export function flattenProjectSessions(projectSessions: Array<{ project: string; sessions: Session[] }>): SessionWithProject[] {
  return sortSessionsByRecent(
    projectSessions.flatMap(({ project, sessions }) => sessions.map((session) => ({ ...session, project }))),
  );
}
