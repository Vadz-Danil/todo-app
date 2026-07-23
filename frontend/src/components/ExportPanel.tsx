import React, { useCallback, useEffect, useState } from 'react';
import {
    Check,
    CircleAlert,
    CircleCheckBig,
    CircleX,
    Copy,
    Download,
    Eye,
    FileJson,
    FileSpreadsheet,
    History,
    KeyRound,
    LoaderCircle,
    Pencil,
    Plus,
    RefreshCw,
    Send,
    Server,
    ShieldCheck,
    Trash2,
    X,
} from 'lucide-react';
import {
    createExportTarget,
    deleteExportTarget,
    downloadAnalyticsExport,
    listDeliveries,
    listExportTargets,
    previewExport,
    pushExport,
    updateExportTarget,
} from '../api/endpoints';
import type { AnalyticsParams, TargetInput } from '../api/endpoints';
import type { ExportDelivery, ExportKind, ExportTarget } from '../types';
import { useToast } from '../context/ToastContext';

interface ExportPanelProps {
    params: AnalyticsParams;
}

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

/** Blob responses hide the JSON error body, so unwrap it before reporting. */
const readBlobError = async (e: unknown, fallback: string): Promise<ApiError> => {
    const base = readApiError(e, fallback);
    if (typeof e === 'object' && e !== null) {
        const data = (e as { response?: { data?: unknown } }).response?.data;
        if (data instanceof Blob) {
            try {
                const parsed: unknown = JSON.parse(await data.text());
                if (typeof parsed === 'object' && parsed !== null) {
                    const server = (parsed as { error?: unknown }).error;
                    if (typeof server === 'string' && server.trim()) {
                        return { ...base, message: server.trim() };
                    }
                }
            } catch {
                /* not JSON — keep the transport-level message */
            }
        }
    }
    return base;
};

const ADHOC = '__adhoc__';

const KINDS: { value: ExportKind; label: string }[] = [
    { value: 'ANALYTICS_SNAPSHOT', label: 'Analytics snapshot' },
    { value: 'TASKS', label: 'Tasks' },
    { value: 'SPRINTS', label: 'Sprints' },
    { value: 'AI_SUMMARY', label: 'AI summary' },
    { value: 'FULL', label: 'Everything' },
];

const KIND_LABELS: Record<ExportKind, string> = {
    ANALYTICS_SNAPSHOT: 'Analytics snapshot',
    TASKS: 'Tasks',
    SPRINTS: 'Sprints',
    AI_SUMMARY: 'AI summary',
    FULL: 'Everything',
};

const GOOD = 'var(--viz-good)';
const BAD = 'var(--viz-critical)';

/** Keeps a status hue legible as text on both surfaces by pulling it toward the ink. */
const inkOf = (color: string): string => `color-mix(in oklab, ${color} 62%, var(--text-h))`;

const relativeTime = (iso?: string): string => {
    if (!iso) return '—';
    const t = new Date(iso).getTime();
    if (Number.isNaN(t)) return iso;
    const seconds = Math.max(0, (Date.now() - t) / 1000);
    if (seconds < 45) return 'just now';
    if (seconds < 3600) return `${Math.round(seconds / 60)}m ago`;
    if (seconds < 86400) return `${Math.round(seconds / 3600)}h ago`;
    if (seconds < 2592000) return `${Math.round(seconds / 86400)}d ago`;
    return new Date(t).toLocaleDateString();
};

const formatBytes = (n: number): string => {
    if (!Number.isFinite(n) || n < 0) return '—';
    if (n < 1024) return `${n} B`;
    if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
    return `${(n / (1024 * 1024)).toFixed(1)} MB`;
};

const formatDuration = (ms: number): string => {
    if (!Number.isFinite(ms) || ms < 0) return '—';
    return ms < 1000 ? `${Math.round(ms)} ms` : `${(ms / 1000).toFixed(1)} s`;
};

let headerRowSeq = 0;

interface HeaderRow {
    id: number;
    key: string;
    value: string;
}

interface TargetForm {
    id?: string;
    name: string;
    url: string;
    secret: string;
    clearSecret: boolean;
    enabled: boolean;
    headers: HeaderRow[];
}

const newHeaderRow = (key = '', value = ''): HeaderRow => ({ id: ++headerRowSeq, key, value });

const emptyForm = (): TargetForm => ({
    name: '',
    url: '',
    secret: '',
    clearSecret: false,
    enabled: true,
    headers: [],
});

const formFor = (t: ExportTarget): TargetForm => ({
    id: t.id,
    name: t.name,
    url: t.url,
    secret: '',
    clearSecret: false,
    enabled: t.enabled,
    headers: Object.entries(t.headers ?? {}).map(([k, v]) => newHeaderRow(k, String(v))),
});

const inputClass =
    'w-full rounded-xl border border-border bg-bg px-3 py-2 text-xs text-text-h outline-none transition placeholder:text-text/40 focus:border-accent-border';

const labelClass = 'font-mono text-[10px] font-medium uppercase tracking-wider text-text/60';

const ghostButton =
    'flex items-center gap-1.5 rounded-xl border border-border bg-hover px-3 py-2 text-xs font-semibold text-text-h transition hover:border-text-h/20 active:scale-95 disabled:opacity-50';

const StatusChip: React.FC<{ ok: boolean; label: string }> = ({ ok, label }) => (
    <span
        className="inline-flex items-center gap-1.5 font-mono text-[10px] font-medium uppercase tracking-wider"
        style={{ color: inkOf(ok ? GOOD : BAD) }}
    >
        {ok ? (
            <CircleCheckBig className="h-3.5 w-3.5 shrink-0" style={{ color: GOOD }} />
        ) : (
            <CircleX className="h-3.5 w-3.5 shrink-0" style={{ color: BAD }} />
        )}
        {label}
    </span>
);

const Stat: React.FC<{ label: string; value: string }> = ({ label, value }) => (
    <div className="min-w-0">
        <div className={labelClass}>{label}</div>
        <div className="mt-0.5 truncate text-xs font-semibold tabular-nums text-text-h">{value}</div>
    </div>
);

const Skeleton: React.FC<{ rows: number }> = ({ rows }) => (
    <div className="space-y-2">
        {Array.from({ length: rows }, (_, i) => (
            <div key={i} className="h-14 animate-pulse rounded-xl border border-border bg-hover/50" />
        ))}
    </div>
);

const InlineError: React.FC<{ title: string; message: string; onRetry?: () => void }> = ({
    title,
    message,
    onRetry,
}) => (
    <div className="flex items-start gap-3 rounded-xl border border-danger/30 bg-danger/10 p-3.5">
        <CircleAlert className="mt-0.5 h-4 w-4 shrink-0 text-danger" />
        <div className="min-w-0 flex-1">
            <p className="text-xs font-semibold text-text-h">{title}</p>
            <p className="mt-1 break-words text-[11px] leading-relaxed text-text">{message}</p>
            {onRetry && (
                <button type="button" onClick={onRetry} className={`${ghostButton} mt-2.5 bg-surface`}>
                    <RefreshCw className="h-3.5 w-3.5" />
                    Try again
                </button>
            )}
        </div>
    </div>
);

export const ExportPanel: React.FC<ExportPanelProps> = ({ params }) => {
    const { showToast } = useToast();

    const [tab, setTab] = useState<'send' | 'destinations'>('send');

    const [targets, setTargets] = useState<ExportTarget[] | null>(null);
    const [targetsError, setTargetsError] = useState<string | null>(null);

    const [deliveries, setDeliveries] = useState<ExportDelivery[] | null>(null);
    const [deliveriesError, setDeliveriesError] = useState<string | null>(null);
    const [deliveriesBusy, setDeliveriesBusy] = useState(false);

    const [kind, setKind] = useState<ExportKind>('ANALYTICS_SNAPSHOT');
    const [destination, setDestination] = useState('');
    const [adhocUrl, setAdhocUrl] = useState('');
    const [adhocSecret, setAdhocSecret] = useState('');

    // Both results are tagged with the window+kind they describe, so changing the
    // filter simply stops showing them instead of needing a reset effect.
    const [previewState, setPreviewState] = useState<{
        key: string;
        text?: string;
        error?: string;
    } | null>(null);
    const [previewBusy, setPreviewBusy] = useState(false);
    const [copied, setCopied] = useState(false);

    const [sending, setSending] = useState(false);
    const [sendState, setSendState] = useState<{
        key: string;
        delivery?: ExportDelivery;
        error?: string;
    } | null>(null);

    const [downloading, setDownloading] = useState<'json' | 'csv' | null>(null);

    const [form, setForm] = useState<TargetForm | null>(null);
    const [formBusy, setFormBusy] = useState(false);
    const [formError, setFormError] = useState<string | null>(null);
    const [rowBusy, setRowBusy] = useState<string | null>(null);
    const [confirmDelete, setConfirmDelete] = useState<string | null>(null);

    const windowKey = `${params.period}|${params.from ?? ''}|${params.to ?? ''}|${
        params.granularity ?? ''
    }|${params.tz ?? ''}`;

    const loadTargets = useCallback(
        (): Promise<void> =>
            listExportTargets()
                .then((list) => {
                    const safe = Array.isArray(list) ? list : [];
                    setTargets(safe);
                    setTargetsError(null);
                    setDestination((prev) => {
                        if (prev && (prev === ADHOC || safe.some((t) => t.id === prev))) return prev;
                        const first = safe.find((t) => t.enabled) ?? safe[0];
                        return first ? first.id : ADHOC;
                    });
                })
                .catch((e: unknown) => {
                    setTargets([]);
                    setTargetsError(readApiError(e, 'Could not load the destinations').message);
                }),
        []
    );

    const loadDeliveries = useCallback(
        (): Promise<void> =>
            listDeliveries(20)
                .then((list) => {
                    setDeliveries(Array.isArray(list) ? list : []);
                    setDeliveriesError(null);
                })
                .catch((e: unknown) => {
                    setDeliveries([]);
                    setDeliveriesError(readApiError(e, 'Could not load the delivery log').message);
                })
                .finally(() => setDeliveriesBusy(false)),
        []
    );

    useEffect(() => {
        void loadTargets();
        void loadDeliveries();
    }, [loadTargets, loadDeliveries]);

    const targetList = targets ?? [];
    const selected = targetList.find((t) => t.id === destination);
    const isAdhoc = destination === ADHOC;

    const payloadKey = `${windowKey}|${kind}`;
    const activePreview = previewState?.key === payloadKey ? previewState : null;
    const preview = activePreview?.text ?? null;
    const previewError = activePreview?.error ?? null;
    const activeSend = sendState?.key === payloadKey ? sendState : null;
    const delivery = activeSend?.delivery ?? null;
    const sendError = activeSend?.error ?? null;

    const runPreview = async () => {
        const key = payloadKey;
        setPreviewBusy(true);
        setPreviewState(null);
        try {
            const data = await previewExport(params, kind);
            setPreviewState({ key, text: JSON.stringify(data ?? null, null, 2) });
        } catch (e) {
            setPreviewState({ key, error: readApiError(e, 'Could not build the preview').message });
        } finally {
            setPreviewBusy(false);
        }
    };

    const copyPreview = async () => {
        if (!preview) return;
        try {
            if (!navigator.clipboard) throw new Error('clipboard unavailable');
            await navigator.clipboard.writeText(preview);
            setCopied(true);
            window.setTimeout(() => setCopied(false), 1600);
        } catch {
            showToast('The clipboard is blocked in this browser — select the JSON manually', 'error');
        }
    };

    const send = async () => {
        const key = payloadKey;
        if (isAdhoc && !adhocUrl.trim()) {
            setSendState({ key, error: 'Enter the URL to POST the payload to.' });
            return;
        }
        if (!isAdhoc && !selected) {
            setSendState({ key, error: 'Pick a saved destination, or choose “Ad-hoc URL…”.' });
            return;
        }
        setSending(true);
        setSendState(null);
        try {
            const result = await pushExport({
                ...params,
                kind,
                target_id: isAdhoc ? undefined : selected?.id,
                url: isAdhoc ? adhocUrl.trim() : undefined,
                secret: isAdhoc && adhocSecret.trim() ? adhocSecret.trim() : undefined,
            });
            setSendState({ key, delivery: result });
            if (result.status === 'SUCCESS') {
                showToast('Payload delivered', 'success');
            } else {
                showToast('The receiver rejected the payload', 'error');
            }
        } catch (e) {
            // The server rejects private/loopback targets unless EXPORT_ALLOW_PRIVATE_TARGETS
            // is on, and that reason only exists in its message — never swallow it.
            setSendState({
                key,
                error: readApiError(e, 'The push failed before it left the server').message,
            });
        } finally {
            setSending(false);
            setDeliveriesBusy(true);
            void loadDeliveries();
            void loadTargets();
        }
    };

    const download = async (format: 'json' | 'csv') => {
        setDownloading(format);
        try {
            const blob = await downloadAnalyticsExport(params, format);
            const href = URL.createObjectURL(blob);
            const anchor = document.createElement('a');
            anchor.href = href;
            anchor.download = `taskflow-analytics-${params.period}-${new Date()
                .toISOString()
                .slice(0, 10)}.${format}`;
            document.body.appendChild(anchor);
            anchor.click();
            anchor.remove();
            URL.revokeObjectURL(href);
            showToast(`${format.toUpperCase()} export downloaded`, 'success');
        } catch (e) {
            const info = await readBlobError(e, 'The download failed');
            showToast(info.message, 'error');
        } finally {
            setDownloading(null);
        }
    };

    const submitForm = async (e: React.FormEvent<HTMLFormElement>) => {
        e.preventDefault();
        if (!form) return;
        if (!form.name.trim() || !form.url.trim()) {
            setFormError('A name and a URL are both required.');
            return;
        }
        setFormBusy(true);
        setFormError(null);

        const headers: Record<string, string> = {};
        form.headers.forEach((row) => {
            const key = row.key.trim();
            if (key) headers[key] = row.value;
        });

        const base: TargetInput = {
            name: form.name.trim(),
            url: form.url.trim(),
            headers,
            enabled: form.enabled,
        };

        try {
            if (form.id) {
                const patch: Partial<TargetInput> = { ...base };
                if (form.secret.trim()) patch.secret = form.secret.trim();
                else if (form.clearSecret) patch.secret = null;
                await updateExportTarget(form.id, patch);
                showToast('Destination updated', 'success');
            } else {
                await createExportTarget({
                    ...base,
                    secret: form.secret.trim() ? form.secret.trim() : undefined,
                });
                showToast('Destination saved', 'success');
            }
            setForm(null);
            await loadTargets();
        } catch (err) {
            setFormError(readApiError(err, 'Could not save the destination').message);
        } finally {
            setFormBusy(false);
        }
    };

    const toggleTarget = async (t: ExportTarget) => {
        setRowBusy(t.id);
        try {
            await updateExportTarget(t.id, { enabled: !t.enabled });
            await loadTargets();
        } catch (e) {
            showToast(readApiError(e, 'Could not update the destination').message, 'error');
        } finally {
            setRowBusy(null);
        }
    };

    const removeTarget = async (id: string) => {
        setRowBusy(id);
        try {
            await deleteExportTarget(id);
            setConfirmDelete(null);
            if (form?.id === id) setForm(null);
            await loadTargets();
            showToast('Destination deleted', 'success');
        } catch (e) {
            showToast(readApiError(e, 'Could not delete the destination').message, 'error');
        } finally {
            setRowBusy(null);
        }
    };

    const patchForm = (patch: Partial<TargetForm>) =>
        setForm((prev) => (prev ? { ...prev, ...patch } : prev));

    return (
        <section className="rounded-2xl border border-border bg-surface p-5 shadow-(--shadow)">
            <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="flex min-w-0 items-center gap-2.5">
                    <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl border border-border bg-hover text-info">
                        <Server className="h-4 w-4" />
                    </div>
                    <div className="min-w-0">
                        <h3 className="text-sm font-semibold text-text-h">Export</h3>
                        <p className="truncate font-mono text-[10px] uppercase tracking-wider text-text/60">
                            push this window to your own server
                        </p>
                    </div>
                </div>

                <div
                    className="flex items-center rounded-xl border border-border bg-hover p-0.5"
                    role="tablist"
                    aria-label="Export panel sections"
                >
                    {([
                        { id: 'send', label: 'Send' },
                        { id: 'destinations', label: 'Destinations' },
                    ] as const).map((t) => (
                        <button
                            key={t.id}
                            type="button"
                            role="tab"
                            aria-selected={tab === t.id}
                            onClick={() => setTab(t.id)}
                            className={`rounded-lg px-3 py-1.5 text-xs font-semibold transition active:scale-95 ${
                                tab === t.id ? 'bg-text-h text-bg' : 'text-text hover:text-text-h'
                            }`}
                        >
                            {t.label}
                        </button>
                    ))}
                </div>
            </div>

            {tab === 'send' ? (
                <div className="mt-4 space-y-4">
                    <div className="grid gap-3 sm:grid-cols-2">
                        <label className="block">
                            <span className={labelClass}>Payload</span>
                            <select
                                value={kind}
                                onChange={(e) => setKind(e.target.value as ExportKind)}
                                className={`${inputClass} mt-1.5`}
                            >
                                {KINDS.map((k) => (
                                    <option key={k.value} value={k.value}>
                                        {k.label}
                                    </option>
                                ))}
                            </select>
                        </label>

                        <label className="block">
                            <span className={labelClass}>Destination</span>
                            <select
                                value={destination}
                                onChange={(e) => setDestination(e.target.value)}
                                className={`${inputClass} mt-1.5`}
                            >
                                {destination === '' && (
                                    <option value="" disabled>
                                        {targets === null ? 'Loading…' : 'Select a destination'}
                                    </option>
                                )}
                                {targetList.map((t) => (
                                    <option key={t.id} value={t.id}>
                                        {t.name}
                                        {t.enabled ? '' : ' (disabled)'}
                                    </option>
                                ))}
                                <option value={ADHOC}>Ad-hoc URL…</option>
                            </select>
                        </label>
                    </div>

                    {isAdhoc ? (
                        <div className="grid gap-3 rounded-xl border border-border bg-hover/40 p-3.5 sm:grid-cols-2">
                            <label className="block">
                                <span className={labelClass}>URL</span>
                                <input
                                    type="url"
                                    inputMode="url"
                                    value={adhocUrl}
                                    onChange={(e) => setAdhocUrl(e.target.value)}
                                    placeholder="https://example.com/hooks/taskflow"
                                    className={`${inputClass} mt-1.5 font-mono`}
                                />
                            </label>
                            <label className="block">
                                <span className={labelClass}>Secret (optional)</span>
                                <input
                                    type="password"
                                    value={adhocSecret}
                                    onChange={(e) => setAdhocSecret(e.target.value)}
                                    placeholder="signs the request when set"
                                    autoComplete="off"
                                    className={`${inputClass} mt-1.5 font-mono`}
                                />
                            </label>
                            <p className="text-[11px] leading-relaxed text-text/70 sm:col-span-2">
                                Loopback and private addresses are refused by the server unless it
                                runs with <span className="rounded bg-code-bg px-1 py-0.5 font-mono text-[10px] text-text-h">EXPORT_ALLOW_PRIVATE_TARGETS=true</span>.
                                Nothing is saved — this URL is used for this one send.
                            </p>
                        </div>
                    ) : (
                        selected && (
                            <div className="flex flex-wrap items-center gap-2 rounded-xl border border-border bg-hover/40 px-3.5 py-2.5">
                                <span className="truncate font-mono text-[11px] text-text" title={selected.url}>
                                    {selected.url}
                                </span>
                                {selected.has_secret && (
                                    <span
                                        className="inline-flex items-center gap-1 font-mono text-[10px] uppercase tracking-wider"
                                        style={{ color: inkOf(GOOD) }}
                                    >
                                        <ShieldCheck className="h-3 w-3" style={{ color: GOOD }} />
                                        signed
                                    </span>
                                )}
                            </div>
                        )
                    )}

                    <div className="flex flex-wrap items-center gap-2">
                        <button
                            type="button"
                            onClick={() => void send()}
                            disabled={sending || destination === ''}
                            className="flex items-center gap-2 rounded-xl bg-success px-4 py-2 text-xs font-bold text-bg shadow-md shadow-success/10 transition hover:bg-success/90 active:scale-95 disabled:opacity-50"
                        >
                            {sending ? (
                                <LoaderCircle className="h-3.5 w-3.5 animate-spin" />
                            ) : (
                                <Send className="h-3.5 w-3.5" />
                            )}
                            {sending ? 'Sending…' : 'Send now'}
                        </button>

                        <button
                            type="button"
                            onClick={() => void runPreview()}
                            disabled={previewBusy}
                            className={ghostButton}
                        >
                            {previewBusy ? (
                                <LoaderCircle className="h-3.5 w-3.5 animate-spin" />
                            ) : (
                                <Eye className="h-3.5 w-3.5" />
                            )}
                            Preview payload
                        </button>

                        <button
                            type="button"
                            onClick={() => void download('json')}
                            disabled={downloading !== null}
                            className={ghostButton}
                            title="Download the analytics dashboard for this window as JSON"
                        >
                            {downloading === 'json' ? (
                                <LoaderCircle className="h-3.5 w-3.5 animate-spin" />
                            ) : (
                                <FileJson className="h-3.5 w-3.5" />
                            )}
                            <span className="hidden sm:inline">Download</span> JSON
                        </button>

                        <button
                            type="button"
                            onClick={() => void download('csv')}
                            disabled={downloading !== null}
                            className={ghostButton}
                            title="Download the analytics dashboard for this window as CSV"
                        >
                            {downloading === 'csv' ? (
                                <LoaderCircle className="h-3.5 w-3.5 animate-spin" />
                            ) : (
                                <FileSpreadsheet className="h-3.5 w-3.5" />
                            )}
                            <span className="hidden sm:inline">Download</span> CSV
                        </button>

                        <span className="ml-auto hidden items-center gap-1.5 text-[10px] text-text/50 sm:flex">
                            <Download className="h-3 w-3" />
                            downloads always use the analytics snapshot
                        </span>
                    </div>

                    {sendError && <InlineError title="The push failed" message={sendError} />}

                    {delivery && (
                        <div
                            className={`rounded-xl border p-3.5 ${
                                delivery.status === 'SUCCESS'
                                    ? 'border-success/30 bg-success/10'
                                    : 'border-danger/30 bg-danger/10'
                            }`}
                        >
                            <div className="flex flex-wrap items-center justify-between gap-2">
                                <StatusChip
                                    ok={delivery.status === 'SUCCESS'}
                                    label={delivery.status}
                                />
                                <span
                                    className="max-w-full truncate font-mono text-[10px] text-text/60"
                                    title={delivery.url}
                                >
                                    {delivery.url}
                                </span>
                            </div>
                            <div className="mt-3 grid grid-cols-2 gap-3 sm:grid-cols-4">
                                <Stat
                                    label="HTTP"
                                    value={delivery.status_code ? String(delivery.status_code) : '—'}
                                />
                                <Stat label="Attempts" value={String(delivery.attempts)} />
                                <Stat label="Duration" value={formatDuration(delivery.duration_ms)} />
                                <Stat label="Payload" value={formatBytes(delivery.payload_size)} />
                            </div>
                            {delivery.error && (
                                <p className="mt-3 break-words border-t border-border/60 pt-2.5 text-[11px] leading-relaxed text-text">
                                    {delivery.error}
                                </p>
                            )}
                        </div>
                    )}

                    {previewError && (
                        <InlineError
                            title="Preview failed"
                            message={previewError}
                            onRetry={() => void runPreview()}
                        />
                    )}

                    {previewBusy && !preview && (
                        <div className="h-24 animate-pulse rounded-xl border border-border bg-hover/50" />
                    )}

                    {preview && (
                        <div className="rounded-xl border border-border bg-code-bg">
                            <div className="flex items-center justify-between gap-2 border-b border-border px-3 py-2">
                                <span className={labelClass}>
                                    {KIND_LABELS[kind] || kind} payload
                                </span>
                                <button
                                    type="button"
                                    onClick={() => void copyPreview()}
                                    className="flex items-center gap-1.5 rounded-lg border border-border bg-surface px-2 py-1 text-[10px] font-semibold text-text-h transition hover:border-text-h/20 active:scale-95"
                                >
                                    {copied ? (
                                        <Check className="h-3 w-3 text-success" />
                                    ) : (
                                        <Copy className="h-3 w-3" />
                                    )}
                                    {copied ? 'Copied' : 'Copy'}
                                </button>
                            </div>
                            <pre className="max-h-80 overflow-auto px-3 py-2.5 font-mono text-[11px] leading-relaxed text-text-h">
                                {preview}
                            </pre>
                        </div>
                    )}
                </div>
            ) : (
                <div className="mt-4 space-y-3">
                    {targets === null ? (
                        <Skeleton rows={2} />
                    ) : targetsError ? (
                        <InlineError
                            title="Could not load the destinations"
                            message={targetsError}
                            onRetry={() => void loadTargets()}
                        />
                    ) : targetList.length === 0 ? (
                        <div className="rounded-xl border border-dashed border-border p-5 text-center">
                            <div className="mx-auto flex h-10 w-10 items-center justify-center rounded-xl border border-border bg-hover text-info">
                                <Server className="h-4 w-4" />
                            </div>
                            <p className="mx-auto mt-3 max-w-sm text-xs leading-relaxed text-text">
                                No saved destinations yet. Add one to push this window on demand
                                without retyping the URL each time.
                            </p>
                        </div>
                    ) : (
                        targetList.map((t) => {
                            const ok =
                                typeof t.last_status === 'number' &&
                                t.last_status >= 200 &&
                                t.last_status < 300;
                            return (
                                <div
                                    key={t.id}
                                    className="rounded-xl border border-border bg-hover/40 p-3.5"
                                >
                                    <div className="flex flex-wrap items-start justify-between gap-3">
                                        <div className="min-w-0 flex-1">
                                            <div className="flex flex-wrap items-center gap-2">
                                                <span className="truncate text-xs font-semibold text-text-h">
                                                    {t.name}
                                                </span>
                                                {t.has_secret && (
                                                    <span
                                                        className="inline-flex items-center gap-1 font-mono text-[10px] uppercase tracking-wider"
                                                        style={{ color: inkOf(GOOD) }}
                                                        title="Requests to this destination carry an HMAC signature"
                                                    >
                                                        <ShieldCheck
                                                            className="h-3 w-3"
                                                            style={{ color: GOOD }}
                                                        />
                                                        signed
                                                    </span>
                                                )}
                                            </div>

                                            <p
                                                className="mt-1 truncate font-mono text-[10px] text-text/70"
                                                title={t.url}
                                            >
                                                {t.url}
                                            </p>

                                            <div className="mt-1.5 flex flex-wrap items-center gap-2">
                                                {t.last_sent_at ? (
                                                    <>
                                                        <StatusChip
                                                            ok={ok}
                                                            label={ok ? 'delivered' : 'failed'}
                                                        />
                                                        <span className="font-mono text-[10px] tabular-nums text-text/50">
                                                            {t.last_status ?? '—'} ·{' '}
                                                            {relativeTime(t.last_sent_at)}
                                                        </span>
                                                    </>
                                                ) : (
                                                    <span className="font-mono text-[10px] uppercase tracking-wider text-text/50">
                                                        never sent
                                                    </span>
                                                )}
                                            </div>

                                            {t.last_error && (
                                                <p
                                                    className="mt-1 truncate text-[10px] text-danger"
                                                    title={t.last_error}
                                                >
                                                    {t.last_error}
                                                </p>
                                            )}
                                        </div>

                                        <div className="flex shrink-0 items-center gap-1.5">
                                            <button
                                                type="button"
                                                role="switch"
                                                aria-checked={t.enabled}
                                                aria-label={`${t.enabled ? 'Disable' : 'Enable'} ${t.name}`}
                                                title={t.enabled ? 'Enabled' : 'Disabled'}
                                                disabled={rowBusy === t.id}
                                                onClick={() => void toggleTarget(t)}
                                                className={`relative h-5 w-9 shrink-0 rounded-full border transition active:scale-95 disabled:opacity-50 ${
                                                    t.enabled
                                                        ? 'border-success/40 bg-success/25'
                                                        : 'border-border bg-hover'
                                                }`}
                                            >
                                                <span
                                                    className={`absolute top-[3px] h-3 w-3 rounded-full transition-all ${
                                                        t.enabled
                                                            ? 'left-[19px] bg-success'
                                                            : 'left-[3px] bg-text/50'
                                                    }`}
                                                />
                                            </button>

                                            <button
                                                type="button"
                                                onClick={() => {
                                                    setForm(formFor(t));
                                                    setFormError(null);
                                                }}
                                                aria-label={`Edit ${t.name}`}
                                                className="rounded-xl border border-border bg-surface p-2 text-text transition hover:text-text-h active:scale-95"
                                            >
                                                <Pencil className="h-3.5 w-3.5" />
                                            </button>

                                            <button
                                                type="button"
                                                onClick={() => setConfirmDelete(t.id)}
                                                aria-label={`Delete ${t.name}`}
                                                className="rounded-xl border border-border bg-surface p-2 text-text transition hover:border-danger/40 hover:bg-danger/10 hover:text-danger active:scale-95"
                                            >
                                                <Trash2 className="h-3.5 w-3.5" />
                                            </button>
                                        </div>
                                    </div>

                                    {confirmDelete === t.id && (
                                        <div className="mt-3 flex flex-wrap items-center justify-between gap-2 rounded-xl border border-danger/30 bg-danger/10 px-3 py-2">
                                            <span className="text-[11px] text-text-h">
                                                Delete “{t.name}”? Past deliveries stay in the log.
                                            </span>
                                            <div className="flex items-center gap-2">
                                                <button
                                                    type="button"
                                                    onClick={() => setConfirmDelete(null)}
                                                    className="rounded-lg px-2.5 py-1 text-[11px] font-semibold text-text hover:text-text-h"
                                                >
                                                    Cancel
                                                </button>
                                                <button
                                                    type="button"
                                                    disabled={rowBusy === t.id}
                                                    onClick={() => void removeTarget(t.id)}
                                                    className="rounded-lg bg-danger px-2.5 py-1 text-[11px] font-bold text-bg transition active:scale-95 disabled:opacity-50"
                                                >
                                                    Delete
                                                </button>
                                            </div>
                                        </div>
                                    )}
                                </div>
                            );
                        })
                    )}

                    {form ? (
                        <form
                            onSubmit={(e) => void submitForm(e)}
                            className="space-y-3 rounded-xl border border-border bg-hover/40 p-3.5"
                        >
                            <div className="flex items-center justify-between gap-2">
                                <span className={labelClass}>
                                    {form.id ? 'Edit destination' : 'New destination'}
                                </span>
                                <button
                                    type="button"
                                    onClick={() => setForm(null)}
                                    aria-label="Close the form"
                                    className="rounded-lg p-1 text-text transition hover:text-text-h"
                                >
                                    <X className="h-3.5 w-3.5" />
                                </button>
                            </div>

                            <div className="grid gap-3 sm:grid-cols-2">
                                <label className="block">
                                    <span className={labelClass}>Name</span>
                                    <input
                                        value={form.name}
                                        onChange={(e) => patchForm({ name: e.target.value })}
                                        placeholder="Ops webhook"
                                        className={`${inputClass} mt-1.5`}
                                    />
                                </label>
                                <label className="block">
                                    <span className={labelClass}>URL</span>
                                    <input
                                        type="url"
                                        inputMode="url"
                                        value={form.url}
                                        onChange={(e) => patchForm({ url: e.target.value })}
                                        placeholder="https://example.com/hooks/taskflow"
                                        className={`${inputClass} mt-1.5 font-mono`}
                                    />
                                </label>
                            </div>

                            <label className="block">
                                <span className={labelClass}>
                                    Secret {form.id ? '(leave blank to keep the current one)' : '(optional)'}
                                </span>
                                <input
                                    type="password"
                                    value={form.secret}
                                    autoComplete="new-password"
                                    onChange={(e) =>
                                        patchForm({ secret: e.target.value, clearSecret: false })
                                    }
                                    placeholder="signs every request with HMAC-SHA256"
                                    className={`${inputClass} mt-1.5 font-mono`}
                                />
                            </label>

                            {form.id && (
                                <label className="flex items-center gap-2 text-[11px] text-text">
                                    <input
                                        type="checkbox"
                                        checked={form.clearSecret}
                                        disabled={form.secret.trim() !== ''}
                                        onChange={(e) => patchForm({ clearSecret: e.target.checked })}
                                        className="h-3.5 w-3.5 accent-[var(--danger)]"
                                    />
                                    Remove the stored secret and send unsigned
                                </label>
                            )}

                            <div>
                                <div className="flex items-center justify-between gap-2">
                                    <span className={labelClass}>Custom headers</span>
                                    <button
                                        type="button"
                                        onClick={() =>
                                            patchForm({ headers: [...form.headers, newHeaderRow()] })
                                        }
                                        className="flex items-center gap-1 rounded-lg border border-border bg-surface px-2 py-1 text-[10px] font-semibold text-text-h transition hover:border-text-h/20 active:scale-95"
                                    >
                                        <Plus className="h-3 w-3" />
                                        Add row
                                    </button>
                                </div>

                                {form.headers.length === 0 ? (
                                    <p className="mt-1.5 text-[11px] text-text/60">
                                        None — the exporter only sends its own headers.
                                    </p>
                                ) : (
                                    <div className="mt-1.5 space-y-2">
                                        {form.headers.map((row) => (
                                            <div key={row.id} className="flex items-center gap-2">
                                                <input
                                                    value={row.key}
                                                    onChange={(e) =>
                                                        patchForm({
                                                            headers: form.headers.map((r) =>
                                                                r.id === row.id
                                                                    ? { ...r, key: e.target.value }
                                                                    : r
                                                            ),
                                                        })
                                                    }
                                                    placeholder="X-Api-Key"
                                                    className={`${inputClass} font-mono`}
                                                />
                                                <input
                                                    value={row.value}
                                                    onChange={(e) =>
                                                        patchForm({
                                                            headers: form.headers.map((r) =>
                                                                r.id === row.id
                                                                    ? { ...r, value: e.target.value }
                                                                    : r
                                                            ),
                                                        })
                                                    }
                                                    placeholder="value"
                                                    className={`${inputClass} font-mono`}
                                                />
                                                <button
                                                    type="button"
                                                    aria-label="Remove this header"
                                                    onClick={() =>
                                                        patchForm({
                                                            headers: form.headers.filter(
                                                                (r) => r.id !== row.id
                                                            ),
                                                        })
                                                    }
                                                    className="shrink-0 rounded-xl border border-border bg-surface p-2 text-text transition hover:border-danger/40 hover:text-danger active:scale-95"
                                                >
                                                    <X className="h-3.5 w-3.5" />
                                                </button>
                                            </div>
                                        ))}
                                    </div>
                                )}
                            </div>

                            <label className="flex items-center gap-2 text-[11px] text-text">
                                <input
                                    type="checkbox"
                                    checked={form.enabled}
                                    onChange={(e) => patchForm({ enabled: e.target.checked })}
                                    className="h-3.5 w-3.5 accent-[var(--success)]"
                                />
                                Enabled
                            </label>

                            {formError && (
                                <p className="break-words rounded-xl border border-danger/30 bg-danger/10 p-2.5 text-[11px] text-text-h">
                                    {formError}
                                </p>
                            )}

                            <div className="flex items-center justify-end gap-2">
                                <button
                                    type="button"
                                    onClick={() => setForm(null)}
                                    className="rounded-xl px-3 py-1.5 text-xs font-semibold text-text hover:text-text-h"
                                >
                                    Cancel
                                </button>
                                <button
                                    type="submit"
                                    disabled={formBusy}
                                    className="flex items-center gap-1.5 rounded-xl bg-success px-4 py-2 text-xs font-bold text-bg shadow-md shadow-success/10 transition hover:bg-success/90 active:scale-95 disabled:opacity-50"
                                >
                                    {formBusy ? (
                                        <LoaderCircle className="h-3.5 w-3.5 animate-spin" />
                                    ) : (
                                        <Check className="h-3.5 w-3.5" strokeWidth={3} />
                                    )}
                                    {form.id ? 'Save changes' : 'Add destination'}
                                </button>
                            </div>
                        </form>
                    ) : (
                        <button
                            type="button"
                            onClick={() => {
                                setForm(emptyForm());
                                setFormError(null);
                            }}
                            className={`${ghostButton} w-full justify-center border-dashed`}
                        >
                            <Plus className="h-3.5 w-3.5" />
                            Add destination
                        </button>
                    )}
                </div>
            )}

            <div className="mt-5 flex items-start gap-2 border-t border-border pt-4">
                <KeyRound className="mt-0.5 h-3.5 w-3.5 shrink-0 text-text/50" />
                <p className="text-[11px] leading-relaxed text-text/70">
                    Payloads are POSTed as JSON. When a secret is set the request also carries{' '}
                    <span className="rounded bg-code-bg px-1 py-0.5 font-mono text-[10px] text-text-h">
                        X-TaskFlow-Signature
                    </span>{' '}
                    — an HMAC-SHA256 over{' '}
                    <span className="rounded bg-code-bg px-1 py-0.5 font-mono text-[10px] text-text-h">
                        timestamp.body
                    </span>{' '}
                    — so the receiver can verify the request came from you and has not been replayed.
                </p>
            </div>

            <div className="mt-5">
                <div className="flex items-center justify-between gap-2">
                    <span className="flex items-center gap-1.5">
                        <History className="h-3.5 w-3.5 text-text/50" />
                        <span className={labelClass}>Recent deliveries</span>
                    </span>
                    <button
                        type="button"
                        onClick={() => {
                            setDeliveriesBusy(true);
                            void loadDeliveries();
                        }}
                        disabled={deliveriesBusy}
                        aria-label="Refresh the delivery log"
                        className="rounded-xl border border-border bg-hover p-1.5 text-text transition hover:text-text-h active:scale-95 disabled:opacity-50"
                    >
                        <RefreshCw
                            className={`h-3.5 w-3.5 ${deliveriesBusy ? 'animate-spin' : ''}`}
                        />
                    </button>
                </div>

                <div className="mt-2">
                    {deliveries === null ? (
                        <Skeleton rows={3} />
                    ) : deliveriesError ? (
                        <InlineError
                            title="Could not load the delivery log"
                            message={deliveriesError}
                            onRetry={() => void loadDeliveries()}
                        />
                    ) : deliveries.length === 0 ? (
                        <p className="rounded-xl border border-dashed border-border p-4 text-center text-[11px] text-text">
                            Nothing has been pushed yet. Deliveries appear here with their HTTP
                            result the moment you send.
                        </p>
                    ) : (
                        <div className="overflow-x-auto">
                            <table className="w-full min-w-[560px] border-collapse text-left">
                                <thead>
                                    <tr className="border-b border-border">
                                        <th className={`${labelClass} pb-2 pr-3 font-medium`}>Kind</th>
                                        <th className={`${labelClass} pb-2 pr-3 font-medium`}>Target</th>
                                        <th className={`${labelClass} pb-2 pr-3 font-medium`}>Status</th>
                                        <th className={`${labelClass} pb-2 pr-3 font-medium`}>HTTP</th>
                                        <th className={`${labelClass} pb-2 font-medium`}>When</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {deliveries.map((d) => (
                                        <tr
                                            key={d.id}
                                            className="border-b border-border/50 last:border-b-0"
                                        >
                                            <td className="py-2 pr-3 text-[11px] text-text-h">
                                                {KIND_LABELS[d.kind] || d.kind}
                                            </td>
                                            <td className="py-2 pr-3">
                                                <span
                                                    className="block max-w-[220px] truncate font-mono text-[11px] text-text"
                                                    title={d.error ? `${d.url} — ${d.error}` : d.url}
                                                >
                                                    {d.url}
                                                </span>
                                            </td>
                                            <td className="py-2 pr-3">
                                                <StatusChip
                                                    ok={d.status === 'SUCCESS'}
                                                    label={d.status}
                                                />
                                            </td>
                                            <td className="py-2 pr-3 font-mono text-[11px] tabular-nums text-text">
                                                {d.status_code ?? '—'}
                                            </td>
                                            <td className="py-2 font-mono text-[11px] text-text/60">
                                                {relativeTime(d.created_at)}
                                            </td>
                                        </tr>
                                    ))}
                                </tbody>
                            </table>
                        </div>
                    )}
                </div>
            </div>
        </section>
    );
};
