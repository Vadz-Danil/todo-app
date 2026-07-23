import React, { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';

/* ============================== tokens ==============================
   Every colour below is a design token from src/index.css. Charts pass
   these strings straight into SVG paint attributes, which resolve them
   as CSS so the light/dark swap happens without a re-render.
   ==================================================================== */

export const VIZ = {
    /** Categorical slots, assigned in FIXED order. Never cycled. */
    cat: ['var(--viz-1)', 'var(--viz-2)', 'var(--viz-3)', 'var(--viz-4)'],
    catVars: ['--viz-1', '--viz-2', '--viz-3', '--viz-4'],
    /** Sequential blue ramp, light -> dark. Magnitude only. */
    seq: [
        'var(--viz-seq-100)',
        'var(--viz-seq-200)',
        'var(--viz-seq-300)',
        'var(--viz-seq-400)',
        'var(--viz-seq-500)',
        'var(--viz-seq-600)',
        'var(--viz-seq-700)',
    ],
    /** Ordinal ramp: one hue, monotone lightness. For ordered categories only. */
    ord: ['var(--viz-ord-1)', 'var(--viz-ord-2)', 'var(--viz-ord-3)', 'var(--viz-ord-4)'],
    ordVars: ['--viz-ord-1', '--viz-ord-2', '--viz-ord-3', '--viz-ord-4'],
    grid: 'var(--viz-grid)',
    axis: 'var(--viz-axis)',
    muted: 'var(--viz-muted)',
    good: 'var(--viz-good)',
    warning: 'var(--viz-warning)',
    serious: 'var(--viz-serious)',
    critical: 'var(--viz-critical)',
    surface: 'var(--surface)',
    ink: 'var(--text-h)',
} as const;

/** The 2px surface-coloured gap that separates touching marks. */
export const GAP = 2;
export const TICK_FONT = 10;

/* ============================ measurement =========================== */

/**
 * Width of the element the ref is attached to, kept live by a
 * ResizeObserver. Charts render `width="100%"` with a viewBox sized to
 * this number, so one viewBox unit stays one CSS pixel and text never
 * scales with the container.
 */
export const useElementWidth = (): [React.RefObject<HTMLDivElement | null>, number] => {
    const ref = useRef<HTMLDivElement | null>(null);
    const [width, setWidth] = useState(0);

    useLayoutEffect(() => {
        const el = ref.current;
        if (!el) return;

        setWidth(el.getBoundingClientRect().width);

        const ro = new ResizeObserver((entries) => {
            const next = entries[0]?.contentRect.width ?? 0;
            setWidth((prev) => (Math.abs(prev - next) < 0.5 ? prev : next));
        });
        ro.observe(el);
        return () => ro.disconnect();
    }, []);

    return [ref, width];
};

/** Bumps whenever ThemeContext flips the `dark` class on <html>. */
export const useThemeTick = (): number => {
    const [tick, setTick] = useState(0);

    useEffect(() => {
        const mo = new MutationObserver(() => setTick((t) => t + 1));
        mo.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] });
        return () => mo.disconnect();
    }, []);

    return tick;
};

const parseHex = (value: string): [number, number, number] | null => {
    const hex = value.trim().replace(/^#/, '');
    if (hex.length !== 3 && hex.length !== 6) return null;
    const full =
        hex.length === 3
            ? hex
                  .split('')
                  .map((c) => c + c)
                  .join('')
            : hex;
    const n = Number.parseInt(full, 16);
    if (Number.isNaN(n)) return null;
    return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
};

const luminance = (rgb: [number, number, number]): number => {
    const [r, g, b] = rgb.map((c) => {
        const s = c / 255;
        return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
    });
    return 0.2126 * r + 0.7152 * g + 0.0722 * b;
};

/**
 * Ink for text set INSIDE a filled mark: white or near-black, picked from
 * the fill's measured luminance so it always clears contrast in both
 * themes. Re-resolves on the theme flip.
 */
export const useInkOn = (cssVars: readonly string[]): string[] => {
    const tick = useThemeTick();
    const key = cssVars.join('|');

    return useMemo(() => {
        if (!key) return [];
        const styles = getComputedStyle(document.documentElement);
        return key.split('|').map((name) => {
            const rgb = parseHex(styles.getPropertyValue(name));
            if (!rgb) return '#ffffff';
            return luminance(rgb) > 0.42 ? '#08060d' : '#ffffff';
        });
    }, [key, tick]);
};

/* ============================== scales ============================== */

const niceStep = (raw: number): number => {
    if (!Number.isFinite(raw) || raw <= 0) return 1;
    const exp = Math.floor(Math.log10(raw));
    const base = 10 ** exp;
    const f = raw / base;
    const snapped = f <= 1 ? 1 : f <= 2 ? 2 : f <= 2.5 ? 2.5 : f <= 5 ? 5 : 10;
    return snapped * base;
};

/** Ticks on clean numbers from 0 to at-or-above `max`. */
export const linearTicks = (max: number, target = 4, integersOnly = false): number[] => {
    if (!Number.isFinite(max) || max <= 0) return [0, 1];
    let step = niceStep(max / target);
    if (integersOnly) step = Math.max(1, Math.round(step));
    const top = Math.ceil(max / step) * step;
    const out: number[] = [];
    for (let v = 0; v <= top + step / 1000; v += step) out.push(Number(v.toFixed(6)));
    return out.length > 1 ? out : [0, top || 1];
};

export interface Margin {
    top: number;
    right: number;
    bottom: number;
    left: number;
}

export interface PlotFrame {
    width: number;
    height: number;
    m: Margin;
    /** Inner plot width / height, in px. */
    iw: number;
    ih: number;
}

export const makeFrame = (width: number, height: number, m: Margin): PlotFrame => ({
    width,
    height,
    m,
    iw: Math.max(0, width - m.left - m.right),
    ih: Math.max(0, height - m.top - m.bottom),
});

/* ============================ formatters ============================ */

export const fmtInt = (n: number): string => Math.round(n).toLocaleString('en-US');

export const fmtNum = (n: number): string =>
    Number.isInteger(n) ? fmtInt(n) : n.toLocaleString('en-US', { maximumFractionDigits: 1 });

/** "3.5h" under a day, "2.1d" at or above one. */
export const fmtHours = (hours: number): string => {
    if (!Number.isFinite(hours) || hours <= 0) return '0h';
    return hours >= 24 ? `${(hours / 24).toFixed(1)}d` : `${hours.toFixed(1)}h`;
};

/** The API sends every rate as a 0..1 fraction. */
export const fmtRate = (fraction: number, digits = 0): string =>
    `${(Number.isFinite(fraction) ? fraction * 100 : 0).toFixed(digits)}%`;

export const fmtCompact = (n: number): string =>
    Math.abs(n) >= 10000 ? `${(n / 1000).toFixed(n % 1000 === 0 ? 0 : 1)}K` : fmtInt(n);

/** Rough advance width; good enough to decide whether a label fits. */
export const estTextWidth = (text: string, fontSize: number): number => text.length * fontSize * 0.58;

/** Indices that still get an x-axis label once the band gets crowded. */
export const thinIndices = (count: number, maxLabels: number): Set<number> => {
    const keep = new Set<number>();
    if (count <= 0) return keep;
    if (maxLabels <= 1) {
        keep.add(count - 1);
        return keep;
    }
    const step = Math.max(1, Math.ceil(count / maxLabels));
    for (let i = 0; i < count; i += step) keep.add(i);
    // The last bucket is always labelled, so clear the slots it would collide with.
    const last = count - 1;
    for (let i = Math.max(0, last - step + 1); i < last; i++) keep.delete(i);
    keep.add(last);
    return keep;
};

/* ============================== marks =============================== */

export interface Corners {
    tl?: boolean;
    tr?: boolean;
    br?: boolean;
    bl?: boolean;
}

/** Rect with per-corner rounding — data-ends round, baseline stays square. */
export const roundedRect = (
    x: number,
    y: number,
    w: number,
    h: number,
    r: number,
    corners: Corners = {}
): string => {
    const rr = Math.max(0, Math.min(r, w / 2, h / 2));
    const tl = corners.tl ? rr : 0;
    const tr = corners.tr ? rr : 0;
    const br = corners.br ? rr : 0;
    const bl = corners.bl ? rr : 0;

    return [
        `M${x + tl},${y}`,
        `H${x + w - tr}`,
        tr ? `A${tr},${tr} 0 0 1 ${x + w},${y + tr}` : '',
        `V${y + h - br}`,
        br ? `A${br},${br} 0 0 1 ${x + w - br},${y + h}` : '',
        `H${x + bl}`,
        bl ? `A${bl},${bl} 0 0 1 ${x},${y + h - bl}` : '',
        `V${y + tl}`,
        tl ? `A${tl},${tl} 0 0 1 ${x + tl},${y}` : '',
        'Z',
    ]
        .filter(Boolean)
        .join(' ');
};

/** Column growing up from a baseline: rounded cap, square foot. */
export const columnPath = (x: number, y: number, w: number, h: number, r = 4): string =>
    roundedRect(x, y, w, h, r, { tl: true, tr: true });

/** Bar growing right from a baseline: rounded tip, square foot. */
export const barPath = (x: number, y: number, w: number, h: number, r = 4): string =>
    roundedRect(x, y, w, h, r, { tr: true, br: true });

/* ============================= chrome =============================== */

/** Solid hairline gridlines one shade off the surface, plus the y ticks. */
export const ChartGrid: React.FC<{
    f: PlotFrame;
    ticks: number[];
    max: number;
    format?: (n: number) => string;
}> = ({ f, ticks, max, format = fmtNum }) => (
    <g aria-hidden="true">
        {ticks.map((t) => {
            const y = f.m.top + f.ih - (max > 0 ? (t / max) * f.ih : 0);
            return (
                <g key={t}>
                    <line
                        x1={f.m.left}
                        x2={f.m.left + f.iw}
                        y1={y}
                        y2={y}
                        stroke={t === 0 ? VIZ.axis : VIZ.grid}
                        strokeWidth={1}
                        shapeRendering="crispEdges"
                    />
                    <text
                        x={f.m.left - 8}
                        y={y + 3.5}
                        textAnchor="end"
                        fontSize={TICK_FONT}
                        fill={VIZ.muted}
                        style={{ fontVariantNumeric: 'tabular-nums' }}
                    >
                        {format(t)}
                    </text>
                </g>
            );
        })}
    </g>
);

/** The x-axis label band. Its height is already inside the chart height. */
export const XTicks: React.FC<{
    f: PlotFrame;
    labels: string[];
    xFor: (i: number) => number;
    slot?: number;
}> = ({ f, labels, xFor, slot = 54 }) => {
    const keep = thinIndices(labels.length, Math.max(1, Math.floor(f.iw / slot)));
    const y = f.m.top + f.ih + 16;

    return (
        <g aria-hidden="true">
            {labels.map((label, i) =>
                keep.has(i) ? (
                    <text
                        key={`${label}-${i}`}
                        x={xFor(i)}
                        y={y}
                        textAnchor="middle"
                        fontSize={TICK_FONT}
                        fill={VIZ.muted}
                        style={{ fontVariantNumeric: 'tabular-nums' }}
                    >
                        {label}
                    </text>
                ) : null
            )}
        </g>
    );
};

/* ============================ interaction =========================== */

export interface TooltipRow {
    label: string;
    value: string;
    color?: string;
}

export interface TooltipState {
    x: number;
    y: number;
    title: string;
    rows: TooltipRow[];
    /** Anchor below the point instead of above it (used where the top clips). */
    below?: boolean;
}

export const ChartTooltip: React.FC<{ tip: TooltipState | null; width: number }> = ({ tip, width }) => {
    if (!tip) return null;

    const half = 78;
    const left = width > half * 2 ? Math.max(half, Math.min(tip.x, width - half)) : tip.x;

    return (
        <div
            className={`pointer-events-none absolute z-20 -translate-x-1/2 rounded-xl border border-border bg-surface px-2.5 py-2 shadow-(--shadow) ${
                tip.below ? '' : '-translate-y-full'
            }`}
            style={{ left, top: tip.below ? tip.y + 8 : Math.max(10, tip.y - 8), minWidth: 132 }}
        >
            <div className="text-[10px] font-mono uppercase tracking-wider text-text">{tip.title}</div>
            <div className="mt-1 flex flex-col gap-0.5">
                {tip.rows.map((r) => (
                    <div key={r.label} className="flex items-center justify-between gap-3">
                        <span className="flex items-center gap-1.5 whitespace-nowrap text-[11px] text-text">
                            {r.color && (
                                <span
                                    className="inline-block h-2 w-2 shrink-0 rounded-full"
                                    style={{ background: r.color }}
                                />
                            )}
                            {r.label}
                        </span>
                        <span className="whitespace-nowrap text-[11px] font-semibold text-text-h">
                            {r.value}
                        </span>
                    </div>
                ))}
            </div>
        </div>
    );
};

/* ============================== legend ============================== */

export interface LegendItem {
    label: string;
    color: string;
    hint?: string;
}

/** Present for two or more series; a single series is named by the title. */
export const Legend: React.FC<{ items: LegendItem[]; className?: string }> = ({
    items,
    className = '',
}) => (
    <ul className={`flex flex-wrap items-center gap-x-4 gap-y-1.5 ${className}`}>
        {items.map((item) => (
            <li key={item.label} className="flex items-center gap-1.5 text-[11px] text-text">
                <span
                    aria-hidden="true"
                    className="h-2.5 w-2.5 shrink-0 rounded-[3px]"
                    style={{ background: item.color }}
                />
                <span className="text-text-h">{item.label}</span>
                {item.hint && <span style={{ color: VIZ.muted }}>{item.hint}</span>}
            </li>
        ))}
    </ul>
);

/** "Less -> more" key for the sequential ramp. */
export const RampLegend: React.FC<{ colors: string[]; from?: string; to?: string }> = ({
    colors,
    from = 'Less',
    to = 'More',
}) => (
    <div className="flex items-center gap-1.5 text-[11px] text-text">
        <span>{from}</span>
        {colors.map((c, i) => (
            <span
                key={i}
                aria-hidden="true"
                className="h-2.5 w-2.5 rounded-[3px]"
                style={{ background: c }}
            />
        ))}
        <span>{to}</span>
    </div>
);

/* ============================== states ============================== */

export const ChartMessage: React.FC<{ children: React.ReactNode; height?: number }> = ({
    children,
    height = 150,
}) => (
    <div
        className="flex items-center justify-center rounded-xl border border-border bg-hover/40 px-4 text-center text-xs text-text"
        style={{ minHeight: height }}
    >
        {children}
    </div>
);

/* =============================== table ============================== */

export interface TableColumn {
    key: string;
    header: string;
    numeric?: boolean;
}

/**
 * The mandatory table view. Two light-mode series colours sit below 3:1,
 * so the same numbers must be readable as text.
 */
export const DataTable: React.FC<{
    columns: TableColumn[];
    rows: React.ReactNode[][];
    caption: string;
    maxHeight?: number;
}> = ({ columns, rows, caption, maxHeight = 340 }) => {
    if (rows.length === 0) return <ChartMessage height={110}>No rows in this range.</ChartMessage>;

    return (
        <div className="overflow-x-auto overflow-y-auto rounded-xl border border-border" style={{ maxHeight }}>
            <table className="w-full min-w-[380px] border-collapse text-xs">
                <caption className="sr-only">{caption}</caption>
                <thead>
                    <tr className="bg-hover/60">
                        {columns.map((c) => (
                            <th
                                key={c.key}
                                scope="col"
                                className={`sticky top-0 z-10 bg-hover px-3 py-2 font-mono text-[10px] font-medium uppercase tracking-wider text-text ${
                                    c.numeric ? 'text-right' : 'text-left'
                                }`}
                            >
                                {c.header}
                            </th>
                        ))}
                    </tr>
                </thead>
                <tbody>
                    {rows.map((row, ri) => (
                        <tr key={ri} className="border-t border-border">
                            {row.map((cell, ci) => (
                                <td
                                    key={columns[ci]?.key ?? ci}
                                    className={`px-3 py-1.5 ${
                                        columns[ci]?.numeric
                                            ? 'text-right tabular-nums text-text-h'
                                            : 'text-left text-text'
                                    }`}
                                >
                                    {cell}
                                </td>
                            ))}
                        </tr>
                    ))}
                </tbody>
            </table>
        </div>
    );
};

/* ============================== helpers ============================= */

/** Pointer x/y in viewBox units (1 unit == 1 CSS px because width is measured). */
export const localPoint = (
    e: React.PointerEvent<SVGSVGElement> | React.MouseEvent<SVGSVGElement>
): { x: number; y: number } => {
    const rect = e.currentTarget.getBoundingClientRect();
    return { x: e.clientX - rect.left, y: e.clientY - rect.top };
};

export const safeArray = <T,>(value: T[] | null | undefined): T[] => (Array.isArray(value) ? value : []);
