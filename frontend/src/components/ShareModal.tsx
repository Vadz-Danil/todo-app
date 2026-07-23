import React, { useCallback, useEffect, useState } from 'react';
import { Check, Copy, Link2, Plus, Trash2, X } from 'lucide-react';
import {
    createShareLink,
    deleteShareLink,
    listShareLinks,
    revokeShareLink,
    shareLinkURL,
} from '../api/endpoints';
import { SHARE_KIND_LABELS, type ShareKind, type ShareLink } from '../types';
import { useToast } from '../context/ToastContext';

interface ShareModalProps {
    isOpen: boolean;
    onClose: () => void;
}

const TTL_OPTIONS: { label: string; days?: number }[] = [
    { label: '7 days', days: 7 },
    { label: '30 days', days: 30 },
    { label: 'Never', days: undefined },
];

/** A link is live when it is neither revoked nor past its expiry. */
const isActive = (link: ShareLink): boolean =>
    !link.revoked_at && (!link.expires_at || new Date(link.expires_at) > new Date());

const formatDate = (iso: string): string =>
    new Date(iso).toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' });

export const ShareModal: React.FC<ShareModalProps> = ({ isOpen, onClose }) => {
    const [links, setLinks] = useState<ShareLink[]>([]);
    const [loading, setLoading] = useState(false);
    const [creating, setCreating] = useState(false);
    const [kind, setKind] = useState<ShareKind>('BOTH');
    const [ttlIndex, setTtlIndex] = useState(0);
    const [label, setLabel] = useState('');
    // The token is returned once, so the freshly minted link is held here to
    // be copied. Reopening the modal will not bring it back.
    const [freshToken, setFreshToken] = useState<string | null>(null);
    const [copied, setCopied] = useState(false);
    const { showToast } = useToast();

    const refresh = useCallback(async () => {
        setLoading(true);
        try {
            setLinks(await listShareLinks());
        } catch {
            showToast('Failed to load share links', 'error');
        } finally {
            setLoading(false);
        }
    }, [showToast]);

    useEffect(() => {
        if (isOpen) void refresh();
    }, [isOpen, refresh]);

    if (!isOpen) return null;

    const copy = async (text: string) => {
        try {
            await navigator.clipboard.writeText(text);
            setCopied(true);
            setTimeout(() => setCopied(false), 2000);
            showToast('Link copied', 'success');
        } catch {
            // Clipboard access can be denied; the field is selectable anyway.
            showToast('Press Ctrl+C to copy the selected link', 'info');
        }
    };

    const handleCreate = async () => {
        setCreating(true);
        try {
            const link = await createShareLink({
                kind,
                label: label.trim() || undefined,
                ttl_days: TTL_OPTIONS[ttlIndex].days,
            });
            setFreshToken(link.token ?? null);
            setLabel('');
            await refresh();
            if (link.token) await copy(shareLinkURL(link.token));
        } catch (err) {
            const msg =
                (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
                'Failed to create the link';
            showToast(msg, 'error');
        } finally {
            setCreating(false);
        }
    };

    const handleRevoke = async (link: ShareLink) => {
        try {
            await revokeShareLink(link.id);
            showToast('Link revoked', 'info');
            await refresh();
        } catch {
            showToast('Failed to revoke the link', 'error');
        }
    };

    const handleDelete = async (link: ShareLink) => {
        try {
            await deleteShareLink(link.id);
            await refresh();
        } catch {
            showToast('Failed to delete the link', 'error');
        }
    };

    return (
        <div className="fixed inset-0 z-50 flex items-end justify-center bg-zinc-950/80 p-0 backdrop-blur-md sm:items-center sm:p-4">
            <div className="max-h-[90vh] w-full max-w-lg overflow-y-auto rounded-t-3xl border border-zinc-800 bg-zinc-900 p-6 shadow-2xl sm:rounded-2xl">
                <div className="flex items-center justify-between border-b border-zinc-800 pb-4">
                    <div className="flex items-center gap-2.5">
                        <div className="rounded-xl border border-zinc-700 bg-zinc-800 p-2 text-emerald-400">
                            <Link2 className="h-4 w-4" />
                        </div>
                        <h2 className="font-mono text-sm font-bold uppercase text-zinc-100">Share a link</h2>
                    </div>
                    <button
                        onClick={onClose}
                        aria-label="Close"
                        className="rounded-lg p-1 text-zinc-400 hover:bg-zinc-800"
                    >
                        <X className="h-4 w-4" />
                    </button>
                </div>

                <p className="mt-4 text-xs leading-relaxed text-zinc-400">
                    Anyone with the link sees a read-only copy — no account needed. Revoke it whenever
                    you want.
                </p>

                {freshToken && (
                    <div className="mt-4 rounded-xl border border-emerald-500/30 bg-emerald-500/5 p-3">
                        <p className="mb-2 font-mono text-[10px] uppercase tracking-wider text-emerald-400">
                            Copy it now — it is shown only once
                        </p>
                        <div className="flex items-center gap-2">
                            <input
                                readOnly
                                value={shareLinkURL(freshToken)}
                                onFocus={(e) => e.currentTarget.select()}
                                className="w-full min-w-0 truncate rounded-lg border border-zinc-800 bg-zinc-950 px-3 py-2 font-mono text-xs text-zinc-200 outline-none"
                            />
                            <button
                                onClick={() => copy(shareLinkURL(freshToken))}
                                aria-label="Copy link"
                                className="shrink-0 rounded-lg border border-zinc-700 bg-zinc-800 p-2 text-zinc-200 transition hover:bg-zinc-700"
                            >
                                {copied ? (
                                    <Check className="h-4 w-4 text-emerald-400" />
                                ) : (
                                    <Copy className="h-4 w-4" />
                                )}
                            </button>
                        </div>
                    </div>
                )}

                <div className="mt-5 space-y-3 rounded-xl border border-zinc-800 bg-zinc-950/50 p-3">
                    <div>
                        <label className="mb-1.5 block font-mono text-[10px] uppercase tracking-wider text-zinc-500">
                            Shows
                        </label>
                        <div className="flex flex-wrap gap-1.5">
                            {(Object.keys(SHARE_KIND_LABELS) as ShareKind[]).map((k) => (
                                <button
                                    key={k}
                                    onClick={() => setKind(k)}
                                    className={`rounded-lg px-2.5 py-1.5 text-xs font-semibold transition ${
                                        kind === k
                                            ? 'bg-zinc-100 text-zinc-900'
                                            : 'border border-zinc-800 text-zinc-400 hover:bg-zinc-800'
                                    }`}
                                >
                                    {SHARE_KIND_LABELS[k]}
                                </button>
                            ))}
                        </div>
                    </div>

                    <div>
                        <label className="mb-1.5 block font-mono text-[10px] uppercase tracking-wider text-zinc-500">
                            Expires
                        </label>
                        <div className="flex flex-wrap gap-1.5">
                            {TTL_OPTIONS.map((opt, i) => (
                                <button
                                    key={opt.label}
                                    onClick={() => setTtlIndex(i)}
                                    className={`rounded-lg px-2.5 py-1.5 text-xs font-semibold transition ${
                                        ttlIndex === i
                                            ? 'bg-zinc-100 text-zinc-900'
                                            : 'border border-zinc-800 text-zinc-400 hover:bg-zinc-800'
                                    }`}
                                >
                                    {opt.label}
                                </button>
                            ))}
                        </div>
                    </div>

                    <input
                        value={label}
                        onChange={(e) => setLabel(e.target.value)}
                        maxLength={120}
                        placeholder="Label (optional) — e.g. Monday standup"
                        className="w-full rounded-lg border border-zinc-800 bg-zinc-950 px-3 py-2 text-xs text-zinc-100 outline-none transition focus:ring-1 focus:ring-emerald-500"
                    />

                    <button
                        onClick={handleCreate}
                        disabled={creating}
                        className="flex w-full items-center justify-center gap-2 rounded-xl bg-emerald-500 py-2.5 text-xs font-bold text-zinc-950 shadow-lg shadow-emerald-500/10 transition hover:bg-emerald-400 disabled:opacity-50"
                    >
                        <Plus className="h-3.5 w-3.5" />
                        <span>{creating ? 'Creating…' : 'Create link'}</span>
                    </button>
                </div>

                <div className="mt-5">
                    <h3 className="mb-2 font-mono text-[10px] uppercase tracking-wider text-zinc-500">
                        Your links {links.length > 0 && `(${links.length})`}
                    </h3>

                    {loading && <p className="text-xs text-zinc-500">Loading…</p>}

                    {!loading && links.length === 0 && (
                        <p className="text-xs text-zinc-500">No links yet.</p>
                    )}

                    <ul className="space-y-2">
                        {links.map((link) => {
                            const active = isActive(link);
                            return (
                                <li
                                    key={link.id}
                                    className="flex items-center justify-between gap-3 rounded-xl border border-zinc-800 bg-zinc-950/50 px-3 py-2.5"
                                >
                                    <div className="min-w-0">
                                        <p className="truncate text-xs font-semibold text-zinc-200">
                                            {link.label || SHARE_KIND_LABELS[link.kind]}
                                        </p>
                                        <p className="mt-0.5 truncate text-[10px] text-zinc-500">
                                            {active ? (
                                                <>
                                                    {link.view_count} view
                                                    {link.view_count === 1 ? '' : 's'}
                                                    {link.expires_at
                                                        ? ` · until ${formatDate(link.expires_at)}`
                                                        : ' · no expiry'}
                                                </>
                                            ) : link.revoked_at ? (
                                                'Revoked'
                                            ) : (
                                                'Expired'
                                            )}
                                        </p>
                                    </div>

                                    <div className="flex shrink-0 items-center gap-1.5">
                                        {active && (
                                            <button
                                                onClick={() => handleRevoke(link)}
                                                className="rounded-lg border border-zinc-800 px-2 py-1 text-[10px] font-semibold text-zinc-400 transition hover:bg-zinc-800 hover:text-zinc-200"
                                            >
                                                Revoke
                                            </button>
                                        )}
                                        <button
                                            onClick={() => handleDelete(link)}
                                            aria-label="Delete link"
                                            className="rounded-lg p-1.5 text-zinc-500 transition hover:bg-zinc-800 hover:text-red-400"
                                        >
                                            <Trash2 className="h-3.5 w-3.5" />
                                        </button>
                                    </div>
                                </li>
                            );
                        })}
                    </ul>
                </div>
            </div>
        </div>
    );
};
