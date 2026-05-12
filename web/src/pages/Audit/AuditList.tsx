import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Badge, Button, EmptyState, Input, Modal } from '@/components/ui';
import { downloadAuditExport, getAuditEvent, listAuditEvents, type AuditEvent } from '@/api/audit';
import { formatTime } from '@/lib/utils';
import { Download, Search, ShieldAlert } from 'lucide-react';

const eventTone: Record<string, string> = {
  'command.blocked': 'bg-amber-500/10 text-amber-700 dark:text-amber-300',
  'permission.denied': 'bg-red-500/10 text-red-700 dark:text-red-300',
  'permission.approved': 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300',
};

export default function AuditList() {
  const { t } = useTranslation();
  const [items, setItems] = useState<AuditEvent[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [query, setQuery] = useState('');
  const [project, setProject] = useState('');
  const [platform, setPlatform] = useState('');
  const [type, setType] = useState('');
  const [appliedParams, setAppliedParams] = useState({
    q: '',
    project: '',
    platform: '',
    type: '',
  });
  const [selected, setSelected] = useState<AuditEvent | null>(null);
  const [detailOpen, setDetailOpen] = useState(false);
  const [error, setError] = useState('');

  const params = useMemo(() => ({
    q: appliedParams.q || undefined,
    project: appliedParams.project || undefined,
    platform: appliedParams.platform || undefined,
    type: appliedParams.type || undefined,
    limit: '100',
  }), [appliedParams]);

  const fetchData = async (requestParams = params) => {
    setLoading(true);
    setError('');
    try {
      const res = await listAuditEvents(requestParams);
      setItems(res.items || []);
      setTotal(res.total || 0);
    } catch (e: any) {
      setError(e.message || 'failed to load audit events');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchData();
  }, []);

  const applyFilters = async () => {
    const nextAppliedParams = {
      q: query.trim(),
      project: project.trim(),
      platform: platform.trim(),
      type: type.trim(),
    };
    setAppliedParams(nextAppliedParams);
    await fetchData({
      q: nextAppliedParams.q || undefined,
      project: nextAppliedParams.project || undefined,
      platform: nextAppliedParams.platform || undefined,
      type: nextAppliedParams.type || undefined,
      limit: '100',
    });
  };

  const onFilterKeyDown: React.KeyboardEventHandler<HTMLInputElement> = (e) => {
    if (e.key === 'Enter') {
      applyFilters();
    }
  };

  const openDetail = async (id: string) => {
    const ev = await getAuditEvent(id);
    setSelected(ev);
    setDetailOpen(true);
  };

  return (
    <div className="space-y-6 animate-fade-in">
      <div className="flex flex-wrap items-end gap-3">
        <Input label={t('audit.search')} value={query} onChange={(e) => setQuery(e.target.value)} onKeyDown={onFilterKeyDown} placeholder={t('common.search')} className="max-w-xs" />
        <Input label={t('audit.project')} value={project} onChange={(e) => setProject(e.target.value)} onKeyDown={onFilterKeyDown} placeholder="project" className="max-w-xs" />
        <Input label={t('audit.platform')} value={platform} onChange={(e) => setPlatform(e.target.value)} onKeyDown={onFilterKeyDown} placeholder="platform" className="max-w-xs" />
        <Input label={t('audit.type')} value={type} onChange={(e) => setType(e.target.value)} onKeyDown={onFilterKeyDown} placeholder="event type" className="max-w-xs" />
        <div className="flex gap-2 items-end ml-auto">
          <Button variant="primary" onClick={applyFilters}><Search size={14} /> {t('common.search')}</Button>
          <Button variant="secondary" onClick={() => downloadAuditExport(params, 'jsonl')}><Download size={14} /> JSONL</Button>
          <Button variant="secondary" onClick={() => downloadAuditExport(params, 'csv')}><Download size={14} /> CSV</Button>
        </div>
      </div>

      {error && <div className="text-sm text-red-500">{error}</div>}

      {loading ? (
        <div className="text-sm text-gray-400">{t('common.loading')}</div>
      ) : items.length === 0 ? (
        <EmptyState message={t('audit.empty')} icon={ShieldAlert} />
      ) : (
        <div className="overflow-hidden rounded-2xl border border-gray-200/80 dark:border-white/[0.08] bg-white/80 dark:bg-white/[0.02]">
          <div className="px-4 py-3 text-xs text-gray-500 border-b border-gray-200/80 dark:border-white/[0.08]">
            {t('audit.total', { total })}
          </div>
          <div className="divide-y divide-gray-200/80 dark:divide-white/[0.06]">
            {items.map((ev) => (
              <button
                key={ev.id}
                type="button"
                onClick={() => openDetail(ev.id)}
                className="w-full text-left px-4 py-3 hover:bg-gray-50/80 dark:hover:bg-white/[0.04] transition-colors"
              >
                <div className="flex flex-wrap items-center gap-2 text-xs">
                  <span className="text-gray-400">{formatTime(ev.timestamp)}</span>
                  <Badge className="text-xs">{ev.project}</Badge>
                  <Badge className={`text-xs ${eventTone[ev.event_type] || ''}`}>{ev.event_type}</Badge>
                  {ev.platform && <span className="text-gray-500">{ev.platform}</span>}
                  {ev.user_name && <span className="text-gray-500">{ev.user_name}</span>}
                  {ev.command && <span className="font-mono text-gray-500">{ev.command}</span>}
                </div>
                <div className="mt-1 text-sm text-gray-900 dark:text-white truncate">
                  {ev.content || ev.reason || ev.result || ev.session_key || ev.id}
                </div>
              </button>
            ))}
          </div>
        </div>
      )}

      <Modal
        open={detailOpen}
        onClose={() => setDetailOpen(false)}
        title={selected ? selected.event_type : t('audit.detail')}
        className="max-w-3xl"
      >
        {selected && (
          <div className="space-y-3 text-sm">
            <div className="grid grid-cols-2 gap-3 text-gray-700 dark:text-gray-300">
              <div><span className="text-gray-400">ID:</span> {selected.id}</div>
              <div><span className="text-gray-400">Time:</span> {formatTime(selected.timestamp)}</div>
              <div><span className="text-gray-400">Project:</span> {selected.project}</div>
              <div><span className="text-gray-400">Platform:</span> {selected.platform || '-'}</div>
              <div><span className="text-gray-400">User:</span> {selected.user_name || selected.user_id || '-'}</div>
              <div><span className="text-gray-400">Session:</span> {selected.session_key || '-'}</div>
            </div>
            <pre className="max-h-80 overflow-auto rounded-xl bg-gray-950 text-gray-100 p-4 text-xs whitespace-pre-wrap break-words">
{JSON.stringify(selected, null, 2)}
            </pre>
          </div>
        )}
      </Modal>
    </div>
  );
}
