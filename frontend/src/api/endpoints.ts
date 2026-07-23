import { api } from './client';
import type {
    AISummary,
    AnalyticsPeriod,
    Dashboard,
    ExportDelivery,
    ExportKind,
    ExportTarget,
    Granularity,
    PlanningSession,
    Sprint,
    Task,
    TaskInput,
    TaskStatus,
} from '../types';

/** The window every analytics-shaped endpoint accepts. */
export interface AnalyticsParams {
    period: AnalyticsPeriod;
    from?: string;
    to?: string;
    granularity?: Granularity;
    tz?: string;
}

/** The browser's IANA zone, so buckets line up with the user's calendar days. */
export const localTimezone = (): string => {
    try {
        return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
    } catch {
        return 'UTC';
    }
};

const analyticsQuery = (p: AnalyticsParams): Record<string, string> => {
    const q: Record<string, string> = { period: p.period, tz: p.tz ?? localTimezone() };
    if (p.from) q.from = p.from;
    if (p.to) q.to = p.to;
    if (p.granularity) q.granularity = p.granularity;
    return q;
};

/* ------------------------------------ me -------------------------------------- */

export interface Me {
    id: string;
    email: string;
    created_at: string;
}

export const getMe = async (): Promise<Me> => (await api.get<Me>('/api/me')).data;

/* ----------------------------------- tasks ------------------------------------ */

export interface TaskQuery {
    status?: TaskStatus[];
    priority?: string[];
    sprint_id?: string;
    q?: string;
    from?: string;
    to?: string;
}

export const listTasks = async (query: TaskQuery = {}): Promise<Task[]> => {
    const params: Record<string, string> = {};
    if (query.status?.length) params.status = query.status.join(',');
    if (query.priority?.length) params.priority = query.priority.join(',');
    if (query.sprint_id) params.sprint_id = query.sprint_id;
    if (query.q) params.q = query.q;
    if (query.from) params.from = query.from;
    if (query.to) params.to = query.to;

    const { data } = await api.get('/api/tasks', { params });
    return Array.isArray(data) ? data : Array.isArray(data?.tasks) ? data.tasks : [];
};

export const createTask = async (input: TaskInput): Promise<Task> =>
    (await api.post<Task>('/api/tasks', input)).data;

export const getTask = async (id: string): Promise<Task> =>
    (await api.get<Task>(`/api/tasks/${id}`)).data;

export const updateTask = async (id: string, patch: Partial<TaskInput>): Promise<Task> =>
    (await api.patch<Task>(`/api/tasks/${id}`, patch)).data;

export const updateTaskStatus = async (
    id: string,
    status: TaskStatus,
    reviewer?: string | null
): Promise<Task> => {
    const body: Record<string, unknown> = { status };
    if (reviewer !== undefined) body.reviewer = reviewer;
    const { data } = await api.patch(`/api/tasks/${id}/status`, body);
    return data.task ?? data;
};

/**
 * Kanban drag. `after_id` is the card that ends up directly above the moved
 * card, `before_id` the one directly below; omit both to append to the column.
 */
export const moveTask = async (
    id: string,
    status: TaskStatus,
    afterId?: string | null,
    beforeId?: string | null
): Promise<Task> =>
    (
        await api.patch<Task>(`/api/tasks/${id}/move`, {
            status,
            after_id: afterId ?? null,
            before_id: beforeId ?? null,
        })
    ).data;

export const deleteTask = async (id: string): Promise<void> => {
    await api.delete(`/api/tasks/${id}`);
};

export const shareTasks = async (recipientEmail: string): Promise<void> => {
    await api.post('/api/tasks/share', { recipient_email: recipientEmail });
};

/* --------------------------------- analytics ---------------------------------- */

export const getDashboard = async (p: AnalyticsParams): Promise<Dashboard> =>
    (await api.get<Dashboard>('/api/analytics/dashboard', { params: analyticsQuery(p) })).data;

export const analyticsExportUrl = (p: AnalyticsParams, format: 'json' | 'csv'): string => {
    const qs = new URLSearchParams({ ...analyticsQuery(p), format }).toString();
    return `${api.defaults.baseURL ?? ''}/api/analytics/export?${qs}`;
};

/** Downloads the analytics export through axios so the auth header is attached. */
export const downloadAnalyticsExport = async (
    p: AnalyticsParams,
    format: 'json' | 'csv'
): Promise<Blob> => {
    const { data } = await api.get('/api/analytics/export', {
        params: { ...analyticsQuery(p), format },
        responseType: 'blob',
    });
    return data as Blob;
};

/* ------------------------------------- ai ------------------------------------- */

export interface AIStatus {
    enabled: boolean;
    model: string;
}

export const getAIStatus = async (): Promise<AIStatus> =>
    (await api.get<AIStatus>('/api/ai/status')).data;

export type AILang = 'uk' | 'en';

export const generateSummary = async (
    p: AnalyticsParams,
    refresh = false,
    lang: AILang = 'uk'
): Promise<AISummary> =>
    (await api.post<AISummary>('/api/ai/summary', { ...analyticsQuery(p), refresh, lang })).data;

export const listSummaries = async (limit = 20): Promise<AISummary[]> => {
    const { data } = await api.get('/api/ai/summaries', { params: { limit } });
    return data?.summaries ?? [];
};

export interface StartPlanningInput {
    raw_tasks: string[];
    notes?: string;
    horizon_weeks: number;
    capacity_hours_per_week: number;
    starts_on?: string;
    include_backlog?: boolean;
    lang?: AILang;
}

export const startPlanning = async (input: StartPlanningInput): Promise<PlanningSession> =>
    (await api.post<PlanningSession>('/api/ai/planning/sessions', input)).data;

export const listPlanningSessions = async (limit = 20): Promise<PlanningSession[]> => {
    const { data } = await api.get('/api/ai/planning/sessions', { params: { limit } });
    return data?.sessions ?? [];
};

export const getPlanningSession = async (id: string): Promise<PlanningSession> =>
    (await api.get<PlanningSession>(`/api/ai/planning/sessions/${id}`)).data;

export const answerPlanning = async (
    id: string,
    answers: { question_id: string; answer: string }[]
): Promise<PlanningSession> =>
    (await api.post<PlanningSession>(`/api/ai/planning/sessions/${id}/answers`, { answers })).data;

export const generatePlan = async (id: string): Promise<PlanningSession> =>
    (await api.post<PlanningSession>(`/api/ai/planning/sessions/${id}/plan`)).data;

export const commitPlan = async (id: string): Promise<{ sprint: Sprint; tasks: Task[] }> =>
    (await api.post<{ sprint: Sprint; tasks: Task[] }>(`/api/ai/planning/sessions/${id}/commit`))
        .data;

export const deletePlanningSession = async (id: string): Promise<void> => {
    await api.delete(`/api/ai/planning/sessions/${id}`);
};

/* ---------------------------------- sprints ----------------------------------- */

export interface SprintInput {
    name: string;
    goal?: string | null;
    starts_on: string;
    ends_on: string;
    capacity_hours?: number | null;
    status?: string;
}

export const listSprints = async (): Promise<Sprint[]> => {
    const { data } = await api.get('/api/sprints');
    return data?.sprints ?? [];
};

export const getSprint = async (id: string): Promise<Sprint> =>
    (await api.get<Sprint>(`/api/sprints/${id}`)).data;

export const createSprint = async (input: SprintInput): Promise<Sprint> =>
    (await api.post<Sprint>('/api/sprints', input)).data;

export const updateSprint = async (id: string, input: Partial<SprintInput>): Promise<Sprint> =>
    (await api.patch<Sprint>(`/api/sprints/${id}`, input)).data;

export const deleteSprint = async (id: string): Promise<void> => {
    await api.delete(`/api/sprints/${id}`);
};

/* ----------------------------------- export ----------------------------------- */

export interface TargetInput {
    name: string;
    url: string;
    secret?: string | null;
    headers?: Record<string, string>;
    enabled?: boolean;
}

export const listExportTargets = async (): Promise<ExportTarget[]> => {
    const { data } = await api.get('/api/export/targets');
    return data?.targets ?? [];
};

export const createExportTarget = async (input: TargetInput): Promise<ExportTarget> =>
    (await api.post<ExportTarget>('/api/export/targets', input)).data;

export const updateExportTarget = async (
    id: string,
    input: Partial<TargetInput>
): Promise<ExportTarget> => (await api.patch<ExportTarget>(`/api/export/targets/${id}`, input)).data;

export const deleteExportTarget = async (id: string): Promise<void> => {
    await api.delete(`/api/export/targets/${id}`);
};

export interface PushInput extends AnalyticsParams {
    target_id?: string;
    url?: string;
    secret?: string;
    headers?: Record<string, string>;
    kind: ExportKind;
}

export const pushExport = async (input: PushInput): Promise<ExportDelivery> => {
    const { target_id, url, secret, headers, kind, ...params } = input;
    const body: Record<string, unknown> = { kind, ...analyticsQuery(params as AnalyticsParams) };
    if (target_id) body.target_id = target_id;
    if (url) body.url = url;
    if (secret) body.secret = secret;
    if (headers) body.headers = headers;
    return (await api.post<ExportDelivery>('/api/export/push', body)).data;
};

export const previewExport = async (
    p: AnalyticsParams,
    kind: ExportKind
): Promise<unknown> =>
    (await api.get('/api/export/preview', { params: { ...analyticsQuery(p), kind } })).data;

export const listDeliveries = async (limit = 50): Promise<ExportDelivery[]> => {
    const { data } = await api.get('/api/export/deliveries', { params: { limit } });
    return data?.deliveries ?? [];
};
