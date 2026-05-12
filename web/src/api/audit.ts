import api from './client';

export interface AuditEvent {
  id: string;
  timestamp: string;
  project: string;
  event_type: string;
  session_key?: string;
  session_id?: string;
  agent_session_id?: string;
  platform?: string;
  user_id?: string;
  user_name?: string;
  chat_id?: string;
  chat_name?: string;
  command?: string;
  result?: string;
  reason?: string;
  role?: string;
  content?: string;
  content_sha256?: string;
  truncated?: boolean;
  metadata?: Record<string, unknown>;
}

export interface AuditListResult {
  items: AuditEvent[];
  total: number;
  limit: number;
  offset: number;
}

export interface AuditFilters {
  [key: string]: string | undefined;
  project?: string;
  user?: string;
  user_name?: string;
  platform?: string;
  type?: string;
  command?: string;
  result?: string;
  session_key?: string;
  session_id?: string;
  q?: string;
  from?: string;
  to?: string;
  limit?: string;
  offset?: string;
}

export const listAuditEvents = (params: AuditFilters) =>
  api.get<AuditListResult>('/audit/events', params);

export const getAuditEvent = async (id: string) => {
  const data = await api.get<{ event: AuditEvent }>(`/audit/events/${id}`);
  return data.event;
};

export async function downloadAuditExport(params: AuditFilters, format: 'jsonl' | 'csv') {
  const qs = new URLSearchParams({ ...params, format }).toString();
  const res = await fetch(`/api/v1/audit/export?${qs}`, {
    headers: api.getToken() ? { Authorization: `Bearer ${api.getToken()}` } : undefined,
  });
  if (!res.ok) {
    throw new Error(`export failed: ${res.status}`);
  }
  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `audit-export.${format}`;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}
