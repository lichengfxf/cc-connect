import { useEffect, useState, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';
import { MessageSquare, Bot, User, Circle, ArrowRight } from 'lucide-react';
import { Card, EmptyState, Badge } from '@/components/ui';
import { listProjects } from '@/api/projects';
import { listSessions } from '@/api/sessions';
import {
  buildChatSessionHref,
  flattenProjectSessions,
  getSessionDisplayName,
  getSessionSubtitle,
  type SessionWithProject,
} from './sessionViewModel';

function timeAgo(iso: string, t: (k: string) => string): string {
  if (!iso) return '';
  const diff = Date.now() - new Date(iso).getTime();
  const mins = Math.floor(diff / 60000);
  if (mins < 1) return t('sessions.justNow');
  if (mins < 60) return `${mins}m`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h`;
  return `${Math.floor(hours / 24)}d`;
}

export default function ChatList() {
  const { t } = useTranslation();
  const [sessions, setSessions] = useState<SessionWithProject[]>([]);
  const [loading, setLoading] = useState(true);

  const fetchData = useCallback(async () => {
    setLoading(true);
    try {
      const { projects: projs } = await listProjects();
      if (!projs?.length) return setSessions([]);
      const results = await Promise.all(
        projs.map(async (p) => {
          try {
            const { sessions } = await listSessions(p.name);
            return { project: p.name, sessions: sessions || [] };
          } catch {
            return { project: p.name, sessions: [] };
          }
        }),
      );
      setSessions(flattenProjectSessions(results));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchData();
    const handler = () => fetchData();
    window.addEventListener('cc:refresh', handler);
    return () => window.removeEventListener('cc:refresh', handler);
  }, [fetchData]);

  if (loading && sessions.length === 0) {
    return <div className="flex items-center justify-center h-64 text-gray-400 animate-pulse">Loading...</div>;
  }

  return (
    <div className="animate-fade-in space-y-4 ">
      <h2 className="text-lg font-bold text-gray-900 dark:text-white">{t('nav.chat')}</h2>

      {sessions.length === 0 ? (
        <EmptyState message={t('chat.noChats')} icon={MessageSquare} />
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {sessions.map((session) => {
            const hasLive = session.live;
            const lastMsg = session.last_message;
            const ts = session.updated_at || session.created_at || '';

            return (
              <Link key={`${session.project}-${session.id}`} to={buildChatSessionHref(session.project, session.id)}>
                <Card hover className="h-full flex flex-col">
                  <div className="flex items-start justify-between mb-3">
                    <div className="min-w-0 flex items-start gap-2.5">
                      <div className="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-accent/10">
                        <MessageSquare size={16} className="text-accent" />
                      </div>
                      <div className="min-w-0">
                        <div className="flex items-center gap-2">
                          <h3 className="font-semibold text-gray-900 dark:text-white truncate">
                            {getSessionDisplayName(session)}
                          </h3>
                          {hasLive && <Circle size={6} className="fill-emerald-500 text-emerald-500 shrink-0" />}
                        </div>
                        <p className="mt-1 text-[11px] text-gray-400 dark:text-gray-500 truncate">
                          {getSessionSubtitle(session)}
                        </p>
                      </div>
                    </div>
                    <ArrowRight size={16} className="text-gray-300 dark:text-gray-600" />
                  </div>

                  <div className="flex-1 min-h-[2rem] mb-3">
                    {lastMsg ? (
                      <p className="text-xs text-gray-500 dark:text-gray-400 line-clamp-2 leading-relaxed">
                        {lastMsg.role === 'user' ? (
                          <User size={10} className="inline mr-1 -mt-0.5 opacity-60" />
                        ) : (
                          <Bot size={10} className="inline mr-1 -mt-0.5 opacity-60" />
                        )}
                        {lastMsg.content.replace(/\n/g, ' ').slice(0, 120)}
                      </p>
                    ) : (
                      <p className="text-xs text-gray-400 dark:text-gray-500 italic">
                        {t('chat.noMessages')}
                      </p>
                    )}
                  </div>

                  <div className="mt-auto flex items-center justify-between border-t border-gray-100 pt-3 text-xs text-gray-500 dark:border-gray-800 dark:text-gray-400">
                    <div className="flex items-center gap-1.5">
                      {session.agent_type && <Badge className="text-[9px]">{session.agent_type}</Badge>}
                    </div>
                    <div className="flex items-center gap-2 text-[11px]">
                      <span>{session.history_count}</span>
                      <span className="text-gray-300 dark:text-gray-600">•</span>
                      {ts && <span className="text-gray-400">{timeAgo(ts, t)}</span>}
                    </div>
                  </div>
                </Card>
              </Link>
            );
          })}
        </div>
      )}
    </div>
  );
}
