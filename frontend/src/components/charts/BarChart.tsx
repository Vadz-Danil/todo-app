import React, { useState } from 'react';
import {
    ChartGrid,
    ChartMessage,
    ChartTooltip,
    GAP,
    TICK_FONT,
    VIZ,
    barPath,
    columnPath,
    estTextWidth,
    fmtNum,
    linearTicks,
    makeFrame,
    safeArray,
    useElementWidth,
    type TooltipState,
} from './primitives';

export interface BarDatum {
    key: string;
    label: string;
    value: number;
}

export interface BarChartProps {
    data: BarDatum[];
    orientation?: 'vertical' | 'horizontal';
    /** Single series: the card title names it, so no legend is drawn. */
    color?: string;
    seriesLabel: string;
    /** Vertical only — includes the x-axis label band. Horizontal sizes to its rows. */
    height?: number;
    format?: (n: number) => string;
    ariaLabel: string;
    emptyMessage?: string;
}

const MAX_BAR = 24;
const ROW_PITCH = 28;
const ROW_BAR = 16;

/** Category labels never overflow their gutter — they get an ellipsis instead. */
const clampLabel = (text: string, maxWidth: number, fontSize: number): string => {
    if (estTextWidth(text, fontSize) <= maxWidth) return text;
    const chars = Math.max(3, Math.floor(maxWidth / (fontSize * 0.58)) - 1);
    return `${text.slice(0, chars)}…`;
};

export const BarChart: React.FC<BarChartProps> = ({
    data,
    orientation = 'vertical',
    color = VIZ.cat[0],
    seriesLabel,
    height = 230,
    format = fmtNum,
    ariaLabel,
    emptyMessage = 'Nothing to plot in this range.',
}) => {
    const [ref, width] = useElementWidth();
    const [hover, setHover] = useState<number | null>(null);

    const items = safeArray(data);
    const n = items.length;
    const max = items.reduce((m, d) => (Number.isFinite(d.value) && d.value > m ? d.value : m), 0);

    const horizontal = orientation === 'horizontal';
    const chartHeight = horizontal ? Math.max(64, n * ROW_PITCH + 8) : height;

    // The category gutter never eats more than ~38% of a narrow card.
    const labelCap = width > 0 ? Math.max(56, width * 0.38) : 148;
    const labelCol = horizontal
        ? Math.min(148, labelCap, Math.max(64, items.reduce((w, d) => Math.max(w, estTextWidth(d.label, 11)), 0) + 12))
        : 42;
    const valueCol = horizontal
        ? Math.min(72, Math.max(38, items.reduce((w, d) => Math.max(w, estTextWidth(format(d.value), 11)), 0) + 14))
        : 12;

    const ticks = linearTicks(max, 4, Number.isInteger(max));
    // Horizontal rows carry a tip label instead of a value axis, so they scale
    // to the data itself — no nice-tick headroom left unused at the right.
    const top = horizontal ? max : ticks[ticks.length - 1];

    const f = makeFrame(width, chartHeight, {
        top: horizontal ? 4 : 14,
        right: valueCol,
        bottom: horizontal ? 4 : 34,
        left: labelCol,
    });

    const tip: TooltipState | null =
        hover === null || width === 0 || !items[hover]
            ? null
            : horizontal
              ? {
                    x: f.m.left + f.iw / 2,
                    y: f.m.top + hover * ROW_PITCH + ROW_PITCH,
                    below: true,
                    title: items[hover].label,
                    rows: [{ label: seriesLabel, value: format(items[hover].value), color }],
                }
              : {
                    x: f.m.left + ((hover + 0.5) / Math.max(1, n)) * f.iw,
                    y: f.m.top + f.ih - (top > 0 ? (items[hover].value / top) * f.ih : 0),
                    title: items[hover].label,
                    rows: [{ label: seriesLabel, value: format(items[hover].value), color }],
                };

    if (n === 0) return <ChartMessage height={120}>{emptyMessage}</ChartMessage>;

    const slot = n > 0 ? f.iw / n : 0;
    const barW = Math.max(4, Math.min(MAX_BAR, slot - GAP * 2));
    const extreme = items.reduce((best, d, i) => (d.value > (items[best]?.value ?? -1) ? i : best), 0);

    return (
        <div ref={ref} className="relative w-full" style={{ minHeight: chartHeight }}>
            {width > 0 && (
                <svg
                    role="img"
                    aria-label={ariaLabel}
                    width="100%"
                    height={chartHeight}
                    viewBox={`0 0 ${width} ${chartHeight}`}
                    onPointerLeave={() => setHover(null)}
                >
                    {!horizontal && <ChartGrid f={f} ticks={ticks} max={top} format={format} />}

                    {horizontal
                        ? items.map((d, i) => {
                              const y = f.m.top + i * ROW_PITCH;
                              const barY = y + (ROW_PITCH - ROW_BAR) / 2;
                              const w = top > 0 ? Math.max(0, (d.value / top) * f.iw) : 0;
                              return (
                                  <g key={d.key}>
                                      <text
                                          x={f.m.left - 8}
                                          y={barY + ROW_BAR / 2 + 4}
                                          textAnchor="end"
                                          fontSize={11}
                                          fill={VIZ.muted}
                                      >
                                          {clampLabel(d.label, labelCol - 10, 11)}
                                      </text>
                                      {w > 0 && (
                                          <path d={barPath(f.m.left, barY, w, ROW_BAR, 4)} fill={color} />
                                      )}
                                      <text
                                          x={f.m.left + w + 8}
                                          y={barY + ROW_BAR / 2 + 4}
                                          fontSize={11}
                                          fontWeight={600}
                                          fill={VIZ.ink}
                                      >
                                          {format(d.value)}
                                      </text>
                                      <rect
                                          x={f.m.left - labelCol}
                                          y={y}
                                          width={Math.max(1, f.iw + labelCol + valueCol)}
                                          height={ROW_PITCH}
                                          fill="transparent"
                                          onPointerEnter={() => setHover(i)}
                                      />
                                  </g>
                              );
                          })
                        : items.map((d, i) => {
                              const cx = f.m.left + (i + 0.5) * slot;
                              const h = top > 0 ? Math.max(0, (d.value / top) * f.ih) : 0;
                              const y = f.m.top + f.ih - h;
                              return (
                                  <g key={d.key}>
                                      {h > 0 && (
                                          <path d={columnPath(cx - barW / 2, y, barW, h, 4)} fill={color} />
                                      )}
                                      {i === extreme && d.value > 0 && (
                                          <text
                                              x={cx}
                                              y={y - 6}
                                              textAnchor="middle"
                                              fontSize={11}
                                              fontWeight={600}
                                              fill={VIZ.ink}
                                          >
                                              {format(d.value)}
                                          </text>
                                      )}
                                      <text
                                          x={cx}
                                          y={f.m.top + f.ih + 16}
                                          textAnchor="middle"
                                          fontSize={TICK_FONT}
                                          fill={VIZ.muted}
                                      >
                                          {d.label}
                                      </text>
                                      <rect
                                          x={cx - Math.max(12, slot / 2)}
                                          y={f.m.top}
                                          width={Math.max(24, slot)}
                                          height={f.ih}
                                          fill="transparent"
                                          onPointerEnter={() => setHover(i)}
                                      />
                                  </g>
                              );
                          })}
                </svg>
            )}
            <ChartTooltip tip={tip} width={width} />
        </div>
    );
};
