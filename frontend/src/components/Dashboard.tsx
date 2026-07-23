import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
    CheckCircle2,
    CircleAlert,
    Flame,
    Gauge,
    Inbox,
    Lock,
    Plus,
    RefreshCw,
    Timer,
    TriangleAlert,
} from 'lucide-react';
import { getDashboard, localTimezone, type AnalyticsParams } from '../api/endpoints';
import {
    BOARD_STATUSES,
    PRIORITIES,
    PRIORITY_LABELS,
    STATUS_LABELS,
    type AnalyticsPeriod,
    type Dashboard as DashboardData,
    type Delta,
    type Granularity,
} from '../types';
import { useToast } from '../context/ToastContext';
import { AISummaryCard } from './AISummaryCard';
import { ExportPanel } from './ExportPanel';
import { ChartCard } from './charts/ChartCard';
import { LineChart } from './charts/LineChart';
import { AreaChart } from './charts/AreaChart';
import { BarChart } from './charts/BarChart';
import { StackedBar, type StackSegment } from './charts/StackedBar';
import { HEAT_BINS, Heatmap } from './charts/Heatmap';
import { Meter } from './charts/Meter';
import { StatTile, type StatDelta } from './charts/StatTile';
import {
    DataTable,
    Legend,
    RampLegend,
    VIZ,
    fmtHours,
    fmtInt,
    fmtRate,
    safeArray,
} from './charts/primitives';

/* --------------------------------- filters --------------------------------- */

type PeriodKey = 'today' | '7d' | '30d' | '90d' | '6m' | '12m' | 'all' | 'custom';

interface PeriodPill {
    id: PeriodKey;
    label: string;
    period: AnalyticsPeriod;
    granularity: Granularity;
}

const PERIODS: PeriodPill[] = [
    { id: 'today', label: 'Today', period: 'today', granularity: 'day' },
    { id: '7d', label: '7d', period: 'week', granularity: 'day' },
    { id: '30d', label: '30d', period: 'month', granularity: 'day' },
    { id: '90d', label: '90d', period: 'quarter', granularity: 'week' },
    { id: '6m', label: '6m', period: 'half_year', granularity: 'week' },
    { id: '12m', label: '12m', period: 'year', granularity: 'month' },
    { id: 'all', label: 'All', period: 'all_time', granularity: 'month' },
    { id: 'custom', label: 'Custom', period: 'custom', granularity: 'day' },
];

const GRANULARITIES: { id: Granularity; label: string }[] = [
    { id: 'day', label: 'Day' },
    { id: 'week', label: 'Week' },
    { id: 'month', label: 'Month' },
];

const isoDay = (d: Date): string =>
    `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;

const daysAgo = (n: number): string => {
    const d = new Date();
    d.setDate(d.getDate() - n);
    return isoDay(d);
};

/** `change_pct` is absent when there is nothing to compare against. */
const toDelta = (d: Delta | undefined, lowerIsBetter = false): StatDelta | undefined => {
    const change = d?.change_pct;
    if (change === undefined || change === null || !Number.isFinite(change)) return undefined;
    return { changePct: change, improved: lowerIsBetter ? change < 0 : change > 0 };
};

const hasAnyData = (d: DashboardData): boolean =>
    (d.totals?.total_tasks ?? 0) > 0 ||
    safeArray(d.series).some((b) => b.created > 0 || b.completed > 0 || b.open_at_end > 0) ||
    safeArray(d.status_breakdown).some((s) => s.count > 0);

/* -------------------------------- component -------------------------------- */

interface DashboardProps {
    isAuthenticated: boolean;
}

export const Dashboard: React.FC<DashboardProps> = ({ isAuthenticated }) => {
    const { showToast } = useToast();

    const [periodKey, setPeriodKey] = useState<PeriodKey>('30d');
    const [granularity, setGranularity] = useState<Granularity>('day');
    const [from, setFrom] = useState<string>(daysAgo(29));
    const [to, setTo] = useState<string>(isoDay(new Date()));
    const [reloadKey, setReloadKey] = useState(0);

    const [data, setData] = useState<DashboardData | null>(null);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);

    const tz = useMemo(() => localTimezone(), []);
    const pill = PERIODS.find((p) => p.id === periodKey) ?? PERIODS[2];
    const isCustom = periodKey === 'custom';
    const customIncomplete = isCustom && (!from || !to);

    const params = useMemo<AnalyticsParams>(
        () => ({
            period: pill.period,
            granularity,
            tz,
            ...(pill.period === 'custom' ? { from, to } : {}),
        }),
        [pill.period, granularity, tz, from, to]
    );

    const request = useRef(0);

    useEffect(() => {
        if (!isAuthenticated) {
            setData(null);
            setError(null);
            return;
        }
        if (params.period === 'custom' && (!params.from || !params.to)) return;

        const id = ++request.current;
        setLoading(true);

        getDashboard(params)
            .then((next) => {
                if (request.current !== id) return;
                setData(next);
                setError(null);
            })
            .catch(() => {
                if (request.current !== id) return;
                setError('Could not load analytics for this range.');
                showToast('Failed to load analytics', 'error');
            })
            .finally(() => {
                if (request.current === id) setLoading(false);
            });
    }, [params, isAuthenticated, reloadKey, showToast]);

    const choosePeriod = useCallback((next: PeriodPill) => {
        setPeriodKey(next.id);
        setGranularity(next.granularity);
        if (next.id === 'custom') {
            setFrom((prev) => prev || daysAgo(29));
            setTo((prev) => prev || isoDay(new Date()));
        }
    }, []);

    /* ------------------------------ derived data ----------------------------- */

    const totals = data?.totals;
    const comparison = data?.comparison;
    const buckets = safeArray(data?.series);
    const bucketLabels = buckets.map((b) => b.label);
    const cycles = safeArray(data?.cycle_time_series);
    const statusRows = safeArray(data?.status_breakdown);
    const priorityRows = safeArray(data?.priority_breakdown);
    const heatDays = safeArray(data?.heatmap);
    const weekdays = safeArray(data?.weekday_load);
    const reviewers = [...safeArray(data?.reviewers)].sort((a, b) => b.total - a.total);
    const sprints = safeArray(data?.sprint_progress);
    const aging = safeArray(data?.aging_wip);

    const statusByKey = new Map(statusRows.map((s) => [s.status, s]));
    // The board columns are a pipeline, not unrelated categories, so they wear
    // the ordinal ramp: one hue, light -> dark from To Do to Done. That also
    // keeps a 78%-wide segment from becoming a loud saturated block.
    const statusSegments: StackSegment[] = BOARD_STATUSES.map((status, i) => ({
        key: status,
        label: STATUS_LABELS[status],
        value: statusByKey.get(status)?.count ?? 0,
        color: VIZ.ord[i],
        inkVar: VIZ.ordVars[i],
        valueLabel: fmtRate(statusByKey.get(status)?.percent ?? 0),
    }));

    const priorityByKey = new Map(priorityRows.map((p) => [p.priority, p]));
    const orderedPriorities = PRIORITIES.map((p) => priorityByKey.get(p)).filter(
        (p): p is NonNullable<typeof p> => p !== undefined
    );
    const priorityAxisMax = orderedPriorities.reduce((m, p) => Math.max(m, p.total), 0);

    /* -------------------------------- shells --------------------------------- */

    if (!isAuthenticated) {
        return (
            <div className="mx-auto w-full max-w-[1600px] px-4 py-10 sm:px-6">
                <div className="flex flex-col items-center gap-3 rounded-2xl border border-border bg-surface p-10 text-center shadow-(--shadow)">
                    <Lock className="h-5 w-5 text-text/60" />
                    <h2 className="text-base">Sign in to see your analytics</h2>
                    <p className="max-w-sm text-xs text-text">
                        Flow, cycle time and completion history are computed from your own tasks.
                    </p>
                </div>
            </div>
        );
    }

    const filterRow = (
        <section className="rounded-2xl border border-border bg-surface p-3 shadow-(--shadow) sm:p-4">
            <div className="flex flex-wrap items-center gap-2">
                <div
                    role="group"
                    aria-label="Period"
                    className="flex flex-wrap items-center gap-0.5 rounded-xl border border-border bg-hover/60 p-1"
                >
                    {PERIODS.map((p) => {
                        const active = p.id === periodKey;
                        return (
                            <button
                                key={p.id}
                                type="button"
                                onClick={() => choosePeriod(p)}
                                aria-pressed={active}
                                className={`rounded-lg px-2.5 py-1 text-[11px] font-semibold transition active:scale-95 ${
                                    active
                                        ? 'bg-text-h text-bg shadow-sm'
                                        : 'text-text hover:bg-hover hover:text-text-h'
                                }`}
                            >
                                {p.label}
                            </button>
                        );
                    })}
                </div>

                <div className="flex items-center gap-2 sm:ml-auto">
                    <label className="flex items-center gap-1.5 text-[10px] font-mono uppercase tracking-wider text-text">
                        <span className="hidden sm:inline">Bucket</span>
                        <select
                            value={granularity}
                            onChange={(e) => setGranularity(e.target.value as Granularity)}
                            className="rounded-lg border border-border bg-bg px-2 py-1 text-xs font-mono text-text-h outline-none transition focus:border-accent"
                        >
                            {GRANULARITIES.map((g) => (
                                <option key={g.id} value={g.id}>
                                    {g.label}
                                </option>
                            ))}
                        </select>
                    </label>

                    <button
                        type="button"
                        onClick={() => setReloadKey((k) => k + 1)}
                        className="flex items-center gap-1.5 rounded-xl border border-border bg-hover px-3 py-1.5 text-xs font-semibold text-text-h transition hover:border-text-h/20 active:scale-95"
                    >
                        <RefreshCw className={`h-3.5 w-3.5 ${loading ? 'animate-spin' : ''}`} />
                        <span className="hidden sm:inline">Refresh</span>
                    </button>
                </div>
            </div>

            {isCustom && (
                <div className="mt-3 flex flex-wrap items-center gap-2">
                    <label className="flex items-center gap-1.5 text-[10px] font-mono uppercase tracking-wider text-text">
                        From
                        <input
                            type="date"
                            value={from}
                            max={to || undefined}
                            onChange={(e) => setFrom(e.target.value)}
                            className="rounded-lg border border-border bg-bg px-2 py-1 text-xs font-mono text-text-h outline-none transition focus:border-accent"
                        />
                    </label>
                    <label className="flex items-center gap-1.5 text-[10px] font-mono uppercase tracking-wider text-text">
                        To
                        <input
                            type="date"
                            value={to}
                            min={from || undefined}
                            onChange={(e) => setTo(e.target.value)}
                            className="rounded-lg border border-border bg-bg px-2 py-1 text-xs font-mono text-text-h outline-none transition focus:border-accent"
                        />
                    </label>
                </div>
            )}

            <p className="mt-3 flex flex-wrap items-center gap-x-2 gap-y-1 text-[11px]" style={{ color: VIZ.muted }}>
                <span>{data?.range?.label ?? (customIncomplete ? 'Pick both dates' : 'Resolving range…')}</span>
                <span aria-hidden="true">·</span>
                <span>{data?.range?.granularity ?? granularity} buckets</span>
                <span aria-hidden="true">·</span>
                <span>{data?.range?.timezone ?? tz}</span>
            </p>
        </section>
    );

    const shell = (children: React.ReactNode) => (
        <div className="mx-auto flex w-full max-w-[1600px] flex-col gap-4 px-4 py-5 sm:px-6">
            {filterRow}
            {children}
        </div>
    );

    if (customIncomplete) {
        return shell(
            <section className="rounded-2xl border border-border bg-surface p-8 text-center shadow-(--shadow)">
                <p className="text-sm text-text-h">Choose a start and an end date</p>
                <p className="mt-1 text-xs text-text">The custom range needs both ends before anything is fetched.</p>
            </section>
        );
    }

    if (!data) {
        return shell(
            <section className="rounded-2xl border border-border bg-surface p-8 text-center shadow-(--shadow)">
                {error ? (
                    <>
                        <CircleAlert className="mx-auto h-5 w-5" style={{ color: VIZ.critical }} />
                        <p className="mt-2 text-sm text-text-h">{error}</p>
                        <button
                            type="button"
                            onClick={() => setReloadKey((k) => k + 1)}
                            className="mt-4 rounded-xl border border-border bg-hover px-3 py-1.5 text-xs font-semibold text-text-h transition hover:border-text-h/20 active:scale-95"
                        >
                            Try again
                        </button>
                    </>
                ) : (
                    <p className="text-sm text-text">Loading analytics…</p>
                )}
            </section>
        );
    }

    if (!hasAnyData(data)) {
        return shell(
            <section className="rounded-2xl border border-border bg-surface p-10 text-center shadow-(--shadow)">
                <Inbox className="mx-auto h-6 w-6 text-text/50" />
                <h2 className="mt-3 text-base">Nothing happened in this range</h2>
                <p className="mx-auto mt-1 max-w-md text-xs text-text">
                    {data.range?.label ?? 'The selected window'} holds no created or completed tasks, so there is
                    nothing to plot. Widen the period or start a task on the board.
                </p>
            </section>
        );
    }

    /* --------------------------------- charts -------------------------------- */

    const deltaHint = 'vs prev';

    return shell(
        <>
            {error && (
                <p
                    role="status"
                    className="flex items-center gap-2 rounded-2xl border border-border bg-surface px-4 py-2.5 text-xs shadow-(--shadow)"
                    style={{ color: VIZ.critical }}
                >
                    <CircleAlert className="h-3.5 w-3.5 shrink-0" />
                    {error} Showing the last successful load.
                </p>
            )}

            <div
                className={`grid grid-cols-2 gap-3 sm:grid-cols-4 ${loading ? 'opacity-50' : ''} transition-opacity duration-200`}
            >
                <StatTile
                    className="col-span-2"
                    hero
                    icon={CheckCircle2}
                    label="Completed"
                    value={fmtInt(totals?.completed_in_range ?? 0)}
                    delta={toDelta(comparison?.completed)}
                    deltaHint={deltaHint}
                />
                <StatTile
                    icon={Plus}
                    label="Created"
                    value={fmtInt(totals?.created_in_range ?? 0)}
                    delta={toDelta(comparison?.created)}
                    deltaHint={deltaHint}
                />
                <StatTile
                    icon={Gauge}
                    label="Completion rate"
                    value={fmtRate(totals?.completion_rate ?? 0)}
                    delta={toDelta(comparison?.completion_rate)}
                    deltaHint={deltaHint}
                />
                <StatTile
                    icon={Timer}
                    label="Avg cycle time"
                    value={fmtHours(totals?.avg_cycle_time_hours ?? 0)}
                    /* Lower is better: a drop is an improvement. */
                    delta={toDelta(comparison?.cycle_time, true)}
                    deltaHint={deltaHint}
                />
                <StatTile
                    icon={Inbox}
                    label="Open now"
                    value={fmtInt(totals?.open_now ?? 0)}
                    footnote={`${fmtInt(totals?.in_progress ?? 0)} in progress`}
                />
                <StatTile
                    icon={TriangleAlert}
                    label="Overdue"
                    value={fmtInt(totals?.overdue ?? 0)}
                    footnote={`${fmtInt(totals?.due_soon ?? 0)} due soon`}
                />
                <StatTile
                    icon={Flame}
                    label="Current streak"
                    value={`${fmtInt(totals?.current_streak_days ?? 0)}d`}
                    footnote={`best ${fmtInt(totals?.longest_streak_days ?? 0)}d`}
                />
            </div>

            <div className="mt-4">
                <AISummaryCard params={params} />
            </div>

            <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
                {/* 1. Flow — two counts, so one shared y axis. */}
                <ChartCard
                    className="lg:col-span-2"
                    title="Flow"
                    subtitle="Tasks created against tasks completed per bucket"
                    loading={loading}
                    legend={
                        <Legend
                            items={[
                                { label: 'Created', color: VIZ.cat[0] },
                                { label: 'Completed', color: VIZ.cat[1] },
                            ]}
                        />
                    }
                    table={
                        <DataTable
                            caption="Created and completed tasks per bucket"
                            columns={[
                                { key: 'bucket', header: 'Bucket' },
                                { key: 'created', header: 'Created', numeric: true },
                                { key: 'completed', header: 'Completed', numeric: true },
                                { key: 'open', header: 'Open at end', numeric: true },
                            ]}
                            rows={buckets.map((b) => [b.label, fmtInt(b.created), fmtInt(b.completed), fmtInt(b.open_at_end)])}
                        />
                    }
                >
                    <LineChart
                        ariaLabel="Tasks created and completed per bucket"
                        labels={bucketLabels}
                        height={280}
                        series={[
                            { key: 'created', label: 'Created', color: VIZ.cat[0], values: buckets.map((b) => b.created) },
                            {
                                key: 'completed',
                                label: 'Completed',
                                color: VIZ.cat[1],
                                values: buckets.map((b) => b.completed),
                            },
                        ]}
                    />
                </ChartCard>

                {/* 2. Open backlog */}
                <ChartCard
                    title="Open backlog"
                    subtitle="Tasks still open at the end of each bucket"
                    loading={loading}
                    table={
                        <DataTable
                            caption="Open tasks at the end of each bucket"
                            columns={[
                                { key: 'bucket', header: 'Bucket' },
                                { key: 'open', header: 'Open at end', numeric: true },
                            ]}
                            rows={buckets.map((b) => [b.label, fmtInt(b.open_at_end)])}
                        />
                    }
                >
                    <AreaChart
                        ariaLabel="Open tasks at the end of each bucket"
                        label="Open at end"
                        labels={bucketLabels}
                        values={buckets.map((b) => b.open_at_end)}
                    />
                </ChartCard>

                {/* 3. Status mix — a 100% composition bar. */}
                <ChartCard
                    title="Status mix"
                    subtitle="Share of every task by board column"
                    loading={loading}
                    legend={
                        <Legend
                            items={statusSegments.map((s) => ({
                                label: s.label,
                                color: s.color,
                                hint: `${fmtInt(s.value)} · ${s.valueLabel ?? ''}`,
                            }))}
                        />
                    }
                    table={
                        <DataTable
                            caption="Task count and share by status"
                            columns={[
                                { key: 'status', header: 'Status' },
                                { key: 'count', header: 'Tasks', numeric: true },
                                { key: 'share', header: 'Share', numeric: true },
                            ]}
                            rows={statusSegments.map((s) => [s.label, fmtInt(s.value), s.valueLabel ?? '—'])}
                        />
                    }
                >
                    {/* A column flex still stretches the child to full width, so the
                        bar keeps its measured size while sitting centred. */}
                    <div className="flex h-full min-h-[120px] flex-col justify-center py-4">
                        <StackedBar
                            ariaLabel="Share of tasks by status"
                            segments={statusSegments}
                            barHeight={32}
                            capStart
                            tooltipTitle="Status mix"
                        />
                    </div>
                </ChartCard>

                {/* 4. Priority — part-to-whole per row: one hue, two shades. */}
                <ChartCard
                    title="Priority"
                    subtitle="Done against open, per priority"
                    loading={loading}
                    legend={
                        <Legend
                            items={[
                                { label: 'Done', color: 'var(--viz-seq-500)' },
                                { label: 'Open', color: 'var(--viz-seq-200)' },
                            ]}
                        />
                    }
                    table={
                        <DataTable
                            caption="Task totals by priority"
                            columns={[
                                { key: 'priority', header: 'Priority' },
                                { key: 'total', header: 'Total', numeric: true },
                                { key: 'done', header: 'Done', numeric: true },
                                { key: 'open', header: 'Open', numeric: true },
                                { key: 'overdue', header: 'Overdue', numeric: true },
                                { key: 'rate', header: 'Done rate', numeric: true },
                            ]}
                            rows={orderedPriorities.map((p) => [
                                PRIORITY_LABELS[p.priority],
                                fmtInt(p.total),
                                fmtInt(p.done),
                                fmtInt(p.open),
                                fmtInt(p.overdue),
                                fmtRate(p.completion_rate),
                            ])}
                        />
                    }
                >
                    {orderedPriorities.length === 0 ? (
                        <p className="py-8 text-center text-xs text-text">No tasks carry a priority yet.</p>
                    ) : (
                        <div className="flex flex-col gap-1.5 py-2">
                            {orderedPriorities.map((p) => (
                                <div
                                    key={p.priority}
                                    className="grid grid-cols-[58px_minmax(0,1fr)] items-center gap-2 sm:grid-cols-[72px_minmax(0,1fr)] sm:gap-3"
                                >
                                    <span className="truncate text-[10px] font-mono uppercase tracking-wider text-text">
                                        {PRIORITY_LABELS[p.priority]}
                                    </span>
                                    <StackedBar
                                        ariaLabel={`${PRIORITY_LABELS[p.priority]} tasks, ${p.done} done of ${p.total}`}
                                        tooltipTitle={PRIORITY_LABELS[p.priority]}
                                        axisMax={priorityAxisMax}
                                        barHeight={18}
                                        inlineLabels={false}
                                        endLabel={fmtInt(p.total)}
                                        endHint={fmtRate(p.completion_rate)}
                                        segments={[
                                            {
                                                key: 'done',
                                                label: 'Done',
                                                value: p.done,
                                                color: 'var(--viz-seq-500)',
                                                inkVar: '--viz-seq-500',
                                            },
                                            {
                                                key: 'open',
                                                label: 'Open',
                                                value: p.open,
                                                color: 'var(--viz-seq-200)',
                                                inkVar: '--viz-seq-200',
                                            },
                                        ]}
                                    />
                                </div>
                            ))}
                        </div>
                    )}
                </ChartCard>

                {/* 5. Cycle time — both series in hours, so one axis. */}
                <ChartCard
                    title="Cycle time"
                    subtitle="Median against 90th percentile, per bucket that had completions"
                    loading={loading}
                    legend={
                        <Legend
                            items={[
                                { label: 'Median', color: VIZ.cat[0] },
                                { label: 'p90', color: VIZ.cat[1] },
                            ]}
                        />
                    }
                    table={
                        <DataTable
                            caption="Median and 90th percentile cycle time per bucket"
                            columns={[
                                { key: 'bucket', header: 'Bucket' },
                                { key: 'median', header: 'Median', numeric: true },
                                { key: 'p90', header: 'p90', numeric: true },
                                { key: 'samples', header: 'Samples', numeric: true },
                            ]}
                            rows={cycles.map((c) => [
                                c.label,
                                fmtHours(c.median_hours),
                                fmtHours(c.p90_hours),
                                fmtInt(c.samples),
                            ])}
                        />
                    }
                >
                    <LineChart
                        ariaLabel="Median and 90th percentile cycle time"
                        labels={cycles.map((c) => c.label)}
                        format={fmtHours}
                        emptyMessage="Nothing was completed in this range, so there is no cycle time to plot."
                        series={[
                            {
                                key: 'median',
                                label: 'Median',
                                color: VIZ.cat[0],
                                /* No completions means no cycle time — a gap, not a zero. */
                                values: cycles.map((c) => (c.samples > 0 ? c.median_hours : null)),
                            },
                            {
                                key: 'p90',
                                label: 'p90',
                                color: VIZ.cat[1],
                                values: cycles.map((c) => (c.samples > 0 ? c.p90_hours : null)),
                            },
                        ]}
                    />
                </ChartCard>

                {/* 6. Completion heatmap */}
                <ChartCard
                    className="lg:col-span-2"
                    title="Completion heatmap"
                    subtitle="Tasks completed per calendar day"
                    loading={loading}
                    legend={<RampLegend colors={HEAT_BINS} />}
                    table={
                        <DataTable
                            caption="Tasks completed per day"
                            columns={[
                                { key: 'date', header: 'Date' },
                                { key: 'count', header: 'Completed', numeric: true },
                            ]}
                            rows={heatDays.map((d) => [d.date, fmtInt(d.count)])}
                        />
                    }
                >
                    <Heatmap ariaLabel="Tasks completed per calendar day" days={heatDays} />
                </ChartCard>

                {/* 7. Weekday load */}
                <ChartCard
                    title="Weekday load"
                    subtitle="Tasks completed by day of week"
                    loading={loading}
                    table={
                        <DataTable
                            caption="Tasks completed and created by weekday"
                            columns={[
                                { key: 'day', header: 'Weekday' },
                                { key: 'completed', header: 'Completed', numeric: true },
                                { key: 'created', header: 'Created', numeric: true },
                                { key: 'avg', header: 'Avg / day', numeric: true },
                            ]}
                            rows={weekdays.map((w) => [
                                w.label,
                                fmtInt(w.completed),
                                fmtInt(w.created),
                                w.avg_per_day.toFixed(1),
                            ])}
                        />
                    }
                >
                    <BarChart
                        ariaLabel="Tasks completed by day of week"
                        seriesLabel="Completed"
                        data={weekdays.map((w) => ({ key: String(w.weekday), label: w.label, value: w.completed }))}
                    />
                </ChartCard>

                {/* 8. Reviewers — only when somebody actually reviewed something. */}
                {reviewers.length > 0 && (
                    <ChartCard
                        title="Reviewers"
                        subtitle="Tasks touched by each reviewer"
                        loading={loading}
                        table={
                            <DataTable
                                caption="Tasks per reviewer"
                                columns={[
                                    { key: 'reviewer', header: 'Reviewer' },
                                    { key: 'in_review', header: 'In review', numeric: true },
                                    { key: 'completed', header: 'Completed', numeric: true },
                                    { key: 'total', header: 'Total', numeric: true },
                                ]}
                                rows={reviewers.map((r) => [
                                    r.reviewer,
                                    fmtInt(r.in_review),
                                    fmtInt(r.completed),
                                    fmtInt(r.total),
                                ])}
                            />
                        }
                    >
                        <BarChart
                            ariaLabel="Tasks per reviewer"
                            orientation="horizontal"
                            seriesLabel="Total tasks"
                            data={reviewers.map((r) => ({ key: r.reviewer, label: r.reviewer, value: r.total }))}
                        />
                    </ChartCard>
                )}

                {/* 9. Sprint progress */}
                {sprints.length > 0 && (
                    <ChartCard
                        title="Sprint progress"
                        subtitle="Completion rate per sprint"
                        loading={loading}
                        table={
                            <DataTable
                                caption="Sprint completion"
                                columns={[
                                    { key: 'name', header: 'Sprint' },
                                    { key: 'range', header: 'Range' },
                                    { key: 'done', header: 'Done', numeric: true },
                                    { key: 'total', header: 'Total', numeric: true },
                                    { key: 'rate', header: 'Rate', numeric: true },
                                ]}
                                rows={sprints.map((s) => [
                                    s.name,
                                    `${s.starts_on} → ${s.ends_on}`,
                                    fmtInt(s.done_tasks),
                                    fmtInt(s.total_tasks),
                                    fmtRate(s.completion_rate),
                                ])}
                            />
                        }
                    >
                        <div className="flex flex-col gap-4 py-1">
                            {sprints.map((s) => (
                                <Meter
                                    key={s.sprint_id}
                                    label={s.name}
                                    sublabel={`${s.starts_on} → ${s.ends_on}`}
                                    value={s.completion_rate}
                                    valueLabel={`${fmtInt(s.done_tasks)} / ${fmtInt(s.total_tasks)} done`}
                                />
                            ))}
                        </div>
                    </ChartCard>
                )}

                {/* 10. Ageing WIP — a plain table; there is no chart to toggle. */}
                <section
                    className={`lg:col-span-2 flex min-w-0 flex-col rounded-2xl border border-border bg-surface p-4 shadow-(--shadow) sm:p-5 ${
                        loading ? 'opacity-50' : ''
                    } transition-opacity duration-200`}
                >
                    <h3 className="text-xs font-mono font-medium uppercase tracking-wider text-text">
                        Ageing work in progress
                    </h3>
                    <p className="mt-1 text-xs text-text/80">Open tasks by how long they have been sitting there</p>
                    <div className="mt-3">
                        <DataTable
                            caption="Open tasks by age"
                            columns={[
                                { key: 'title', header: 'Task' },
                                { key: 'status', header: 'Status' },
                                { key: 'priority', header: 'Priority' },
                                { key: 'age', header: 'Age', numeric: true },
                                { key: 'flag', header: 'Due' },
                            ]}
                            rows={aging.map((a) => [
                                <span key="t" className="text-text-h">
                                    {a.title}
                                </span>,
                                STATUS_LABELS[a.status] ?? a.status,
                                PRIORITY_LABELS[a.priority] ?? a.priority,
                                fmtHours(a.age_hours),
                                a.is_overdue ? (
                                    /* Icon AND the word — never colour alone. */
                                    <span
                                        key="f"
                                        className="inline-flex items-center gap-1 font-medium"
                                        style={{ color: VIZ.critical }}
                                    >
                                        <TriangleAlert className="h-3.5 w-3.5" />
                                        Overdue
                                    </span>
                                ) : (
                                    <span key="f" style={{ color: VIZ.muted }}>
                                        On track
                                    </span>
                                ),
                            ])}
                        />
                    </div>
                </section>
            </div>

            <div className="mt-4">
                <ExportPanel params={params} />
            </div>
        </>
    );
};
