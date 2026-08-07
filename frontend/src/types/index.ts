export type TaskStatus = 'TODO' | 'IN_PROGRESS' | 'IN_REVIEW' | 'DONE';
export type TaskPriority = 'LOW' | 'MEDIUM' | 'HIGH' | 'URGENT';

export const BOARD_STATUSES: TaskStatus[] = ['TODO', 'IN_PROGRESS', 'IN_REVIEW', 'DONE'];
export const PRIORITIES: TaskPriority[] = ['URGENT', 'HIGH', 'MEDIUM', 'LOW'];

export const STATUS_LABELS: Record<TaskStatus, string> = {
    TODO: 'To Do',
    IN_PROGRESS: 'In Progress',
    IN_REVIEW: 'In Review',
    DONE: 'Done',
};

export const PRIORITY_LABELS: Record<TaskPriority, string> = {
    URGENT: 'Urgent',
    HIGH: 'High',
    MEDIUM: 'Medium',
    LOW: 'Low',
};

export interface Task {
    id: string;
    user_id?: string;
    title: string;
    description?: string;
    status: TaskStatus;
    priority: TaskPriority;
    reviewer?: string;
    estimate_hours?: number;
    buffer_hours?: number;
    spent_hours?: number;
    blockers?: string;
    sprint_id?: string;
    due_date?: string;
    started_at?: string;
    completed_at?: string;
    position: number;
    created_at: string;
    updated_at?: string;
}

export interface TaskInput {
    title: string;
    description?: string;
    status?: TaskStatus;
    priority?: TaskPriority;
    reviewer?: string | null;
    estimate_hours?: number | null;
    buffer_hours?: number | null;
    spent_hours?: number | null;
    blockers?: string | null;
    sprint_id?: string | null;
    due_date?: string | null;
}

/* ---------------------------------- analytics --------------------------------- */

export type AnalyticsPeriod =
    | 'today'
    | 'week'
    | 'month'
    | 'quarter'
    | 'half_year'
    | 'year'
    | 'all_time'
    | 'custom';

export type Granularity = 'day' | 'week' | 'month';

export interface RangeInfo {
    period: AnalyticsPeriod;
    from: string;
    to: string;
    granularity: Granularity;
    timezone: string;
    label: string;
    days: number;
}

export interface Totals {
    total_tasks: number;
    created_in_range: number;
    completed_in_range: number;
    open_now: number;
    todo: number;
    in_progress: number;
    in_review: number;
    done: number;
    overdue: number;
    due_soon: number;
    blocked: number;
    completion_rate: number;
    avg_cycle_time_hours: number;
    median_cycle_time_hours: number;
    planned_hours: number;
    buffer_hours: number;
    spent_hours: number;
    estimate_accuracy: number;
    avg_completed_per_day: number;
    active_days: number;
    current_streak_days: number;
    longest_streak_days: number;
    best_day?: string;
    best_day_count: number;
}

export interface Delta {
    current: number;
    previous: number;
    change_pct?: number;
}

export interface Comparison {
    previous_from: string;
    previous_to: string;
    created: Delta;
    completed: Delta;
    completion_rate: Delta;
    cycle_time: Delta;
}

export interface TimeBucket {
    bucket_start: string;
    bucket_end: string;
    label: string;
    created: number;
    completed: number;
    open_at_end: number;
    completed_hours: number;
}

export interface StatusBucket {
    status: TaskStatus;
    count: number;
    percent: number;
}

export interface PriorityBucket {
    priority: TaskPriority;
    total: number;
    done: number;
    open: number;
    overdue: number;
    percent: number;
    completion_rate: number;
    estimate_hours: number;
    avg_cycle_hours: number;
}

export interface CycleTimePoint {
    bucket_start: string;
    label: string;
    avg_hours: number;
    median_hours: number;
    p90_hours: number;
    samples: number;
}

export interface DayCount {
    date: string;
    count: number;
}

export interface WeekdayBucket {
    weekday: number;
    label: string;
    completed: number;
    created: number;
    avg_per_day: number;
}

export interface ReviewerBucket {
    reviewer: string;
    in_review: number;
    completed: number;
    total: number;
}

export interface SprintStatsItem {
    sprint_id: string;
    name: string;
    status: SprintStatus;
    starts_on: string;
    ends_on: string;
    total_tasks: number;
    done_tasks: number;
    planned_hours: number;
    completion_rate: number;
}

export interface AgingItem {
    task_id: string;
    title: string;
    status: TaskStatus;
    priority: TaskPriority;
    age_hours: number;
    reviewer?: string;
    is_overdue: boolean;
}

export interface Dashboard {
    range: RangeInfo;
    totals: Totals;
    comparison: Comparison;
    series: TimeBucket[];
    status_breakdown: StatusBucket[];
    priority_breakdown: PriorityBucket[];
    cycle_time_series: CycleTimePoint[];
    heatmap: DayCount[];
    weekday_load: WeekdayBucket[];
    reviewers: ReviewerBucket[];
    sprint_progress: SprintStatsItem[];
    aging_wip: AgingItem[];
    generated_at: string;
}

/* ----------------------------------- sprints ---------------------------------- */

export type SprintStatus = 'PLANNED' | 'ACTIVE' | 'COMPLETED' | 'ARCHIVED';

export interface SprintStats {
    total_tasks: number;
    done_tasks: number;
    planned_hours: number;
    buffer_hours: number;
    committed_hours: number;
    completion_rate: number;
    load_percent: number;
}

export interface Sprint {
    id: string;
    user_id?: string;
    name: string;
    goal?: string;
    starts_on: string;
    ends_on: string;
    capacity_hours?: number;
    status: SprintStatus;
    ai_rationale?: string;
    ai_model?: string;
    created_at: string;
    updated_at?: string;
    tasks?: Task[];
    stats?: SprintStats;
}

/* ---------------------------------- ai planning -------------------------------- */

export type PlanningState = 'COLLECTING' | 'READY' | 'PLANNED' | 'COMMITTED';

export type QuestionTopic =
    | 'BLOCKERS'
    | 'ESTIMATE'
    | 'BUFFER'
    | 'SCOPE'
    | 'PRIORITY'
    | 'DEPENDENCY'
    | 'REVIEW';

export interface PlanningItem {
    ref: string;
    title: string;
    notes?: string;
    priority?: TaskPriority;
    has_blockers?: boolean;
    blockers?: string;
    estimate_hours?: number;
    buffer_hours?: number;
    needs_review?: boolean;
    reviewer?: string;
    depends_on?: string[];
    resolved: boolean;
    existing_task_id?: string;
}

export interface PlanningQuestion {
    id: string;
    item_ref: string;
    topic: QuestionTopic;
    question: string;
    why?: string;
    suggestions?: string[];
    answer?: string;
    answered_at?: string;
}

export interface PlanningMessage {
    role: string;
    content: string;
    at: string;
}

export interface PlannedSubtask {
    title: string;
    estimate_hours: number;
}

export interface PlannedTask {
    ref: string;
    title: string;
    description?: string;
    priority: TaskPriority;
    estimate_hours: number;
    buffer_hours: number;
    blockers?: string;
    needs_review: boolean;
    reviewer?: string;
    depends_on?: string[];
    order: number;
    subtasks?: PlannedSubtask[];
}

export interface PlannedWeek {
    index: number;
    starts_on: string;
    ends_on: string;
    capacity_hours: number;
    load_hours: number;
    focus?: string;
    tasks: PlannedTask[];
}

export interface DeferredItem {
    ref: string;
    title: string;
    reason: string;
}

export interface SprintPlan {
    name: string;
    goal: string;
    starts_on: string;
    ends_on: string;
    capacity_hours: number;
    total_estimate_hours: number;
    total_buffer_hours: number;
    committed_hours: number;
    load_percent: number;
    weeks: PlannedWeek[];
    deferred?: DeferredItem[];
    risks?: string[];
    rationale?: string;
    recommendations?: string[];
}

export interface PlanningPayload {
    items: PlanningItem[];
    questions: PlanningQuestion[];
    messages: PlanningMessage[];
    plan?: SprintPlan;
    notes?: string;
    committed_ids?: string[];
}

export interface PlanningSession {
    id: string;
    user_id?: string;
    state: PlanningState;
    horizon_weeks: number;
    capacity_hours_per_week: number;
    starts_on?: string;
    sprint_id?: string;
    ai_model?: string;
    payload: PlanningPayload;
    created_at: string;
    updated_at: string;
}

/* ---------------------------------- ai summary --------------------------------- */

export interface AIMetricNote {
    label: string;
    value: string;
    comment?: string;
}

export interface AISummaryContent {
    headline: string;
    summary: string;
    highlights?: string[];
    risks?: string[];
    recommendations?: string[];
    focus_next?: string[];
    metrics?: AIMetricNote[];
    trend: string;
    score: number;
}

export interface AISummary {
    id: string;
    period: string;
    range_start: string;
    range_end: string;
    ai_model?: string;
    content: AISummaryContent;
    created_at: string;
    cached: boolean;
}

/* ------------------------------------ export ----------------------------------- */

export type ExportKind =
    | 'ANALYTICS_SNAPSHOT'
    | 'TASKS'
    | 'SPRINTS'
    | 'AI_SUMMARY'
    | 'FULL';

export interface ExportTarget {
    id: string;
    name: string;
    url: string;
    has_secret: boolean;
    headers: Record<string, string>;
    enabled: boolean;
    last_status?: number;
    last_error?: string;
    last_sent_at?: string;
    created_at: string;
    updated_at: string;
}

export interface ExportDelivery {
    id: string;
    target_id?: string;
    url: string;
    kind: ExportKind;
    status: 'SUCCESS' | 'FAILED';
    status_code?: number;
    attempts: number;
    duration_ms: number;
    payload_size: number;
    error?: string;
    created_at: string;
}

export type ShareKind = 'BOARD' | 'DASHBOARD' | 'BOTH';

export const SHARE_KIND_LABELS: Record<ShareKind, string> = {
    BOARD: 'Board only',
    DASHBOARD: 'Dashboard only',
    BOTH: 'Board + dashboard',
};

export interface ShareLink {
    id: string;
    /** Present only in the response that created the link, never again. */
    token?: string;
    kind: ShareKind;
    label: string;
    expires_at?: string;
    revoked_at?: string;
    view_count: number;
    last_viewed_at?: string;
    created_at: string;
}

/** The public projection of a task: no owner identifiers. */
export interface SharedTask {
    id: string;
    title: string;
    description?: string;
    status: TaskStatus;
    priority: TaskPriority;
    reviewer?: string;
    estimate_hours?: number;
    buffer_hours?: number;
    spent_hours?: number;
    blockers?: string;
    due_date?: string;
    completed_at?: string;
    created_at: string;
}

export interface SharedBoardColumn {
    status: TaskStatus;
    tasks: SharedTask[];
}

export interface SharedView {
    kind: ShareKind;
    label: string;
    owner_email: string;
    generated_at: string;
    expires_at?: string;
    columns?: SharedBoardColumn[];
    task_count: number;
    analytics?: Dashboard;
}

export interface Subtask {
    id: string;
    task_id: string;
    title: string;
    done: boolean;
    position: number;
    created_at: string;
    updated_at: string;
}

export interface SubtaskProgress {
    done: number;
    total: number;
}
