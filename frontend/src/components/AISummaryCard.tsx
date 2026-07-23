import React, { useEffect, useState } from 'react';
import {
    Ban,
    CircleAlert,
    CircleCheckBig,
    History,
    Lightbulb,
    LoaderCircle,
    Minus,
    RefreshCw,
    Sparkles,
    Target,
    TrendingDown,
    TrendingUp,
    TriangleAlert,
} from 'lucide-react';
import { generateSummary, getAIStatus } from '../api/endpoints';
import type { AILang, AnalyticsParams } from '../api/endpoints';
import type { AIMetricNote, AISummary } from '../types';
import { useToast } from '../context/ToastContext';

interface AISummaryCardProps {
    params: AnalyticsParams;
}

type IconComponent = React.ComponentType<{ className?: string; style?: React.CSSProperties }>;

interface ApiError {
    status?: number;
    message: string;
}

/** Pulls `{"error": "..."}` out of an axios rejection without ever widening to `any`. */
const readApiError = (e: unknown, fallback: string): ApiError => {
    if (typeof e !== 'object' || e === null) return { message: fallback };
    const shaped = e as { response?: { status?: number; data?: unknown }; message?: unknown };
    const data = shaped.response?.data;
    let message: string | undefined;
    if (typeof data === 'object' && data !== null) {
        const server = (data as { error?: unknown }).error;
        if (typeof server === 'string' && server.trim()) message = server.trim();
    }
    if (!message && typeof shaped.message === 'string' && shaped.message.trim()) {
        message = shaped.message.trim();
    }
    return { status: shaped.response?.status, message: message ?? fallback };
};

type TrendKey = 'IMPROVING' | 'STEADY' | 'DECLINING' | 'AT_RISK';

const TREND_KEYS: TrendKey[] = ['IMPROVING', 'STEADY', 'DECLINING', 'AT_RISK'];

const TRENDS: Record<TrendKey, { word: string; icon: IconComponent; color: string }> = {
    IMPROVING: { word: 'IMPROVING', icon: TrendingUp, color: 'var(--viz-good)' },
    STEADY: { word: 'STEADY', icon: Minus, color: 'var(--viz-muted)' },
    DECLINING: { word: 'DECLINING', icon: TrendingDown, color: 'var(--viz-warning)' },
    AT_RISK: { word: 'AT RISK', icon: TriangleAlert, color: 'var(--viz-critical)' },
};

const trendKeyOf = (raw: string): TrendKey | null => {
    const normalized = raw.trim().toUpperCase().replace(/[\s-]+/g, '_');
    return TREND_KEYS.find((k) => k === normalized) ?? null;
};

/** Meter severity: the fill carries the state, the track is the same hue lightened. */
const scoreColor = (score: number): string => {
    if (score >= 75) return 'var(--viz-good)';
    if (score >= 50) return 'var(--viz-warning)';
    if (score >= 25) return 'var(--viz-serious)';
    return 'var(--viz-critical)';
};

/** Keeps a status hue legible as text on both surfaces by pulling it toward the ink. */
const inkOf = (color: string): string => `color-mix(in oklab, ${color} 62%, var(--text-h))`;
const tintOf = (color: string, pct: number): string =>
    `color-mix(in oklab, ${color} ${pct}%, transparent)`;

const cleanList = (items?: string[]): string[] =>
    Array.isArray(items) ? items.filter((s): s is string => typeof s === 'string' && s.trim() !== '') : [];

const cleanMetrics = (items?: AIMetricNote[]): AIMetricNote[] =>
    Array.isArray(items)
        ? items.filter((m): m is AIMetricNote => !!m && typeof m.label === 'string' && m.label.trim() !== '')
        : [];

const formatStamp = (iso: string): string => {
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return iso;
    return d.toLocaleString(undefined, {
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
    });
};

const PHASES = [
    'Collecting the analytics window…',
    'Sending the window to the model…',
    'The model is writing the read…',
    'Still writing — long windows take longer…',
];

const phaseFor = (seconds: number): string => {
    if (seconds < 3) return PHASES[0];
    if (seconds < 9) return PHASES[1];
    if (seconds < 22) return PHASES[2];
    return PHASES[3];
};

/** One generation attempt, tagged with the window it belongs to. */
interface RunResult {
    key: string;
    seq: number;
    startedAt: number;
    loading: boolean;
    summary: AISummary | null;
    error: ApiError | null;
}

let runSeq = 0;

const SummaryList: React.FC<{
    title: string;
    icon: IconComponent;
    color: string;
    items: string[];
}> = ({ title, icon: Icon, color, items }) => {
    if (items.length === 0) return null;
    return (
        <div className="rounded-xl border border-border bg-hover/40 p-3.5">
            <div className="flex items-center gap-1.5">
                <Icon className="h-3.5 w-3.5 shrink-0" style={{ color }} />
                <span className="font-mono text-[10px] font-medium uppercase tracking-wider text-text/60">
                    {title}
                </span>
                <span className="ml-auto font-mono text-[10px] tabular-nums text-text/50">
                    {items.length}
                </span>
            </div>
            <ul className="mt-2 space-y-1.5">
                {items.map((item, i) => (
                    <li key={`${i}-${item.slice(0, 24)}`} className="flex gap-2 text-[11px] leading-relaxed text-text">
                        <span
                            className="mt-[6px] h-1 w-1 shrink-0 rounded-full"
                            style={{ background: color }}
                        />
                        <span className="min-w-0 break-words">{item}</span>
                    </li>
                ))}
            </ul>
        </div>
    );
};

export const AISummaryCard: React.FC<AISummaryCardProps> = ({ params }) => {
    const { showToast } = useToast();

    const [model, setModel] = useState('');
    const [aiEnabled, setAiEnabled] = useState<boolean | null>(null);
    const [lang, setLang] = useState<AILang>('uk');
    const [result, setResult] = useState<RunResult | null>(null);
    const [elapsed, setElapsed] = useState(0);

    const windowKey = `${params.period}|${params.from ?? ''}|${params.to ?? ''}|${
        params.granularity ?? ''
    }|${params.tz ?? ''}`;

    // A summary describes exactly one window, so anything from another window is
    // simply not shown — no reset effect, and a late response can never surface.
    const active = result && result.key === windowKey ? result : null;
    const loading = active?.loading ?? false;
    const summary = active?.summary ?? null;
    const error = active?.error ?? null;

    useEffect(() => {
        let alive = true;
        getAIStatus()
            .then((s) => {
                if (!alive) return;
                setAiEnabled(s.enabled);
                setModel(typeof s.model === 'string' ? s.model : '');
            })
            .catch(() => {
                if (alive) setAiEnabled(null);
            });
        return () => {
            alive = false;
        };
    }, []);

    const startedAt = active?.startedAt ?? 0;

    useEffect(() => {
        if (!loading || !startedAt) return;
        const id = window.setInterval(
            () => setElapsed(Math.max(0, (Date.now() - startedAt) / 1000)),
            250
        );
        return () => window.clearInterval(id);
    }, [loading, startedAt]);

    const run = async (refresh: boolean) => {
        const seq = ++runSeq;
        setElapsed(0);
        setResult({
            key: windowKey,
            seq,
            startedAt: Date.now(),
            loading: true,
            summary: null,
            error: null,
        });
        try {
            const data = await generateSummary(params, refresh, lang);
            setResult((prev) =>
                prev && prev.seq === seq ? { ...prev, loading: false, summary: data } : prev
            );
        } catch (e) {
            const info = readApiError(e, 'Could not generate the summary');
            setResult((prev) =>
                prev && prev.seq === seq ? { ...prev, loading: false, error: info } : prev
            );
            if (info.status === 503) {
                // AI is switched off server-side — inline only, a toast here would loop.
                setAiEnabled(false);
            } else {
                showToast(info.message, 'error');
            }
        }
    };

    const content = summary?.content;
    const trend = content ? trendKeyOf(content.trend ?? '') : null;
    const trendConf = trend ? TRENDS[trend] : null;
    const TrendIcon = trendConf?.icon;
    const rawScore = typeof content?.score === 'number' && Number.isFinite(content.score) ? content.score : 0;
    const score = Math.max(0, Math.min(100, Math.round(rawScore)));
    const meter = scoreColor(score);

    const highlights = cleanList(content?.highlights);
    const risks = cleanList(content?.risks);
    const recommendations = cleanList(content?.recommendations);
    const focusNext = cleanList(content?.focus_next);
    const metrics = cleanMetrics(content?.metrics);
    const hasLists =
        highlights.length + risks.length + recommendations.length + focusNext.length > 0;

    const progress = Math.min(96, 100 * (1 - Math.exp(-elapsed / 11)));
    const disabled = aiEnabled === false;

    return (
        <section className="rounded-2xl border border-border bg-surface p-5 shadow-(--shadow)">
            <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="flex min-w-0 items-center gap-2.5">
                    <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl border border-border bg-hover text-accent">
                        <Sparkles className="h-4 w-4" />
                    </div>
                    <div className="min-w-0">
                        <h3 className="text-sm font-semibold text-text-h">AI summary</h3>
                        <p className="truncate font-mono text-[10px] uppercase tracking-wider text-text/60">
                            {model || (aiEnabled === false ? 'disabled' : 'model —')}
                        </p>
                    </div>
                </div>

                <div className="flex shrink-0 items-center gap-2">
                    <div
                        className="flex items-center rounded-xl border border-border bg-hover p-0.5"
                        role="group"
                        aria-label="Summary language"
                    >
                        {(['uk', 'en'] as AILang[]).map((code) => (
                            <button
                                key={code}
                                type="button"
                                onClick={() => setLang(code)}
                                aria-pressed={lang === code}
                                title={code === 'uk' ? 'Write the summary in Ukrainian' : 'Write the summary in English'}
                                className={`rounded-lg px-2.5 py-1 font-mono text-[10px] font-bold uppercase tracking-wider transition active:scale-95 ${
                                    lang === code
                                        ? 'bg-text-h text-bg'
                                        : 'text-text hover:text-text-h'
                                }`}
                            >
                                {code}
                            </button>
                        ))}
                    </div>

                    {summary && (
                        <button
                            type="button"
                            onClick={() => void run(true)}
                            disabled={loading || disabled}
                            title="Force a fresh generation. A plain request returns the cached summary while the underlying data is unchanged, because the backend caches by content fingerprint."
                            className="flex items-center gap-1.5 rounded-xl border border-border bg-hover px-3 py-1.5 text-xs font-semibold text-text-h transition hover:border-text-h/20 active:scale-95 disabled:opacity-50"
                        >
                            <RefreshCw className={`h-3.5 w-3.5 ${loading ? 'animate-spin' : ''}`} />
                            <span className="hidden sm:inline">Regenerate</span>
                        </button>
                    )}
                </div>
            </div>

            {error && (
                <div
                    className={`mt-4 flex items-start gap-3 rounded-xl border p-4 ${
                        error.status === 503
                            ? 'border-warning/30 bg-warning/10'
                            : 'border-danger/30 bg-danger/10'
                    }`}
                >
                    {error.status === 503 ? (
                        <Ban className="mt-0.5 h-4 w-4 shrink-0 text-warning" />
                    ) : (
                        <CircleAlert className="mt-0.5 h-4 w-4 shrink-0 text-danger" />
                    )}
                    <div className="min-w-0 flex-1">
                        <p className="text-xs font-semibold text-text-h">
                            {error.status === 503 ? 'AI summaries are switched off' : 'Generation failed'}
                        </p>
                        <p className="mt-1 break-words text-[11px] leading-relaxed text-text">
                            {error.message}
                        </p>
                        <button
                            type="button"
                            onClick={() => void run(false)}
                            disabled={loading}
                            className="mt-2.5 flex items-center gap-1.5 rounded-xl border border-border bg-surface px-3 py-1.5 text-xs font-semibold text-text-h transition hover:border-text-h/20 active:scale-95 disabled:opacity-50"
                        >
                            <RefreshCw className="h-3.5 w-3.5" />
                            {error.status === 503 ? 'Check again' : 'Try again'}
                        </button>
                    </div>
                </div>
            )}

            {loading ? (
                <div className="mt-4 rounded-xl border border-border bg-hover/50 p-4">
                    <div className="flex items-center gap-2">
                        <LoaderCircle className="h-4 w-4 shrink-0 animate-spin text-accent" />
                        <span className="min-w-0 truncate text-xs font-semibold text-text-h">
                            {phaseFor(elapsed)}
                        </span>
                        <span className="ml-auto shrink-0 font-mono text-[10px] tabular-nums text-text/60">
                            {Math.floor(elapsed)}s
                        </span>
                    </div>
                    <div
                        className="mt-3 h-1.5 w-full overflow-hidden rounded-full"
                        style={{ background: tintOf('var(--accent)', 18) }}
                        role="progressbar"
                        aria-label="Generating the AI summary"
                        aria-valuemin={0}
                        aria-valuemax={100}
                        aria-valuenow={Math.round(progress)}
                    >
                        <div
                            className="h-full rounded-r-[4px] bg-accent transition-[width] duration-300 ease-out"
                            style={{ width: `${progress}%` }}
                        />
                    </div>
                    <p className="mt-2 text-[11px] leading-relaxed text-text">
                        This is one model call over the whole window — usually 10 to 30 seconds. You
                        can keep working; the result lands here.
                    </p>
                </div>
            ) : summary && content ? (
                <div className="mt-4 space-y-4">
                    <div>
                        <p className="text-sm font-semibold leading-snug text-text-h sm:text-base">
                            {content.headline?.trim() || 'Summary ready'}
                        </p>

                        <div className="mt-2 flex flex-wrap items-center gap-2">
                            {trendConf && TrendIcon && (
                                <span
                                    className="inline-flex items-center gap-1.5 rounded-lg border px-2 py-0.5 font-mono text-[10px] font-medium uppercase tracking-wider"
                                    style={{
                                        color: inkOf(trendConf.color),
                                        borderColor: tintOf(trendConf.color, 40),
                                        background: tintOf(trendConf.color, 12),
                                    }}
                                >
                                    <TrendIcon
                                        className="h-3.5 w-3.5"
                                        style={{ color: trendConf.color }}
                                    />
                                    {trendConf.word}
                                </span>
                            )}
                            {!trendConf && content.trend && (
                                <span className="inline-flex items-center gap-1.5 rounded-lg border border-border bg-hover px-2 py-0.5 font-mono text-[10px] uppercase tracking-wider text-text">
                                    <Minus className="h-3.5 w-3.5" />
                                    {content.trend}
                                </span>
                            )}

                            {summary.cached ? (
                                <span
                                    className="inline-flex items-center gap-1.5 rounded-lg border border-border bg-hover px-2 py-0.5 font-mono text-[10px] uppercase tracking-wider text-text/70"
                                    title="Served from the cache — the underlying data has not changed since it was written. Use Regenerate to force a fresh call."
                                >
                                    <History className="h-3 w-3" />
                                    cached · {formatStamp(summary.created_at)}
                                </span>
                            ) : (
                                <span className="font-mono text-[10px] uppercase tracking-wider text-text/50">
                                    generated {formatStamp(summary.created_at)}
                                </span>
                            )}
                        </div>
                    </div>

                    <div>
                        <div className="flex items-baseline justify-between gap-3">
                            <span className="font-mono text-[10px] font-medium uppercase tracking-wider text-text/60">
                                Score
                            </span>
                            <span className="text-sm font-semibold text-text-h">
                                {score}
                                <span className="ml-0.5 font-mono text-[10px] text-text/60">/ 100</span>
                            </span>
                        </div>
                        <div
                            className="mt-1.5 h-1.5 w-full overflow-hidden rounded-full"
                            style={{ background: tintOf(meter, 18) }}
                            role="meter"
                            aria-label="Overall score out of 100"
                            aria-valuemin={0}
                            aria-valuemax={100}
                            aria-valuenow={score}
                        >
                            <div
                                className="h-full rounded-r-[4px]"
                                style={{ width: `${score}%`, background: meter }}
                            />
                        </div>
                    </div>

                    {content.summary?.trim() && (
                        <p className="text-xs leading-relaxed text-text sm:text-sm">
                            {content.summary}
                        </p>
                    )}

                    {hasLists && (
                        <div className="grid gap-3 sm:grid-cols-2">
                            <SummaryList
                                title="Highlights"
                                icon={CircleCheckBig}
                                color="var(--viz-good)"
                                items={highlights}
                            />
                            <SummaryList
                                title="Risks"
                                icon={TriangleAlert}
                                color="var(--viz-critical)"
                                items={risks}
                            />
                            <SummaryList
                                title="Recommendations"
                                icon={Lightbulb}
                                color="var(--viz-warning)"
                                items={recommendations}
                            />
                            <SummaryList
                                title="Focus next"
                                icon={Target}
                                color="var(--accent)"
                                items={focusNext}
                            />
                        </div>
                    )}

                    {metrics.length > 0 && (
                        <div>
                            <span className="font-mono text-[10px] font-medium uppercase tracking-wider text-text/60">
                                Metrics
                            </span>
                            <div className="mt-2 grid gap-2 sm:grid-cols-2 xl:grid-cols-3">
                                {metrics.map((m, i) => (
                                    <div
                                        key={`${i}-${m.label}`}
                                        className="rounded-xl border border-border bg-hover/40 p-3"
                                    >
                                        <div className="font-mono text-[10px] uppercase tracking-wider text-text/60">
                                            {m.label}
                                        </div>
                                        <div className="mt-0.5 break-words text-sm font-semibold text-text-h">
                                            {m.value || '—'}
                                        </div>
                                        {m.comment && (
                                            <div className="mt-1 break-words text-[11px] leading-relaxed text-text">
                                                {m.comment}
                                            </div>
                                        )}
                                    </div>
                                ))}
                            </div>
                        </div>
                    )}

                    {!hasLists && metrics.length === 0 && !content.summary?.trim() && (
                        <p className="text-[11px] leading-relaxed text-text">
                            The model returned a headline but no detail for this window — usually a
                            sign there is very little activity in it. Try a wider period.
                        </p>
                    )}
                </div>
            ) : (
                !error && (
                    <div className="mt-4 rounded-xl border border-dashed border-border p-5 text-center sm:p-6">
                        <div className="mx-auto flex h-10 w-10 items-center justify-center rounded-xl border border-border bg-hover text-accent">
                            <Sparkles className="h-4 w-4" />
                        </div>
                        <p className="mx-auto mt-3 max-w-md text-xs leading-relaxed text-text">
                            A written read of the selected window — what moved, what is at risk and
                            what to do next. It is a single model call, so it costs money and takes
                            10 to 30 seconds. Nothing runs until you ask.
                        </p>
                        <button
                            type="button"
                            onClick={() => void run(false)}
                            disabled={disabled}
                            className="mx-auto mt-4 flex items-center gap-2 rounded-xl bg-accent px-4 py-2 text-xs font-bold text-bg shadow-md transition hover:opacity-90 active:scale-95 disabled:opacity-50"
                        >
                            <Sparkles className="h-3.5 w-3.5" />
                            Generate summary
                        </button>
                        {disabled && (
                            <p className="mt-2.5 text-[11px] text-warning">
                                AI features are disabled on the server, so this cannot run right now.
                            </p>
                        )}
                    </div>
                )
            )}
        </section>
    );
};
