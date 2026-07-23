import React, { useState } from 'react';
import {
    ChartTooltip,
    GAP,
    VIZ,
    estTextWidth,
    fmtNum,
    roundedRect,
    safeArray,
    useElementWidth,
    useInkOn,
    type TooltipState,
} from './primitives';

export interface StackSegment {
    key: string;
    label: string;
    value: number;
    color: string;
    /** CSS custom-property name of `color`; picks the ink for an inline label. */
    inkVar: string;
    /** Short readout drawn inside the segment when it fits, e.g. "42%". */
    valueLabel?: string;
}

export interface StackedBarProps {
    segments: StackSegment[];
    /** Value mapped to the full width. Defaults to the sum (a 100% bar). */
    axisMax?: number;
    barHeight?: number;
    /** A composition bar has no free end, so both ends get the 4px radius. */
    capStart?: boolean;
    inlineLabels?: boolean;
    minLabelWidth?: number;
    format?: (v: number) => string;
    tooltipTitle?: string;
    /** Direct label at the bar end, e.g. the row total. */
    endLabel?: string;
    /** Muted note after the end label, e.g. the completion rate. */
    endHint?: string;
    ariaLabel: string;
    emptyMessage?: string;
}

export const StackedBar: React.FC<StackedBarProps> = ({
    segments,
    axisMax,
    barHeight = 24,
    capStart = false,
    inlineLabels = true,
    minLabelWidth = 44,
    format = fmtNum,
    tooltipTitle,
    endLabel,
    endHint,
    ariaLabel,
    emptyMessage = 'No tasks yet.',
}) => {
    const [ref, width] = useElementWidth();
    const [hover, setHover] = useState<number | null>(null);

    const all = safeArray(segments);
    const inks = useInkOn(all.map((s) => s.inkVar));

    const sum = all.reduce((acc, s) => acc + Math.max(0, s.value || 0), 0);
    const denom = axisMax && axisMax > 0 ? axisMax : sum;
    const height = Math.max(28, barHeight + 8);
    const barY = (height - barHeight) / 2;

    // The end label lives inside the SVG, so reserve room for it up front.
    const reserve =
        endLabel || endHint
            ? estTextWidth(endLabel ?? '', 11) + estTextWidth(endHint ?? '', 11) + 24
            : 0;
    const plotW = Math.max(0, width - reserve);

    const visible = all
        .map((s, i) => ({ ...s, ink: inks[i] ?? '#ffffff', index: i }))
        .filter((s) => s.value > 0);

    let cursor = 0;
    const laid = visible.map((s, i) => {
        const raw = denom > 0 ? (s.value / denom) * plotW : 0;
        const isLast = i === visible.length - 1;
        const w = Math.max(1, raw - (isLast ? 0 : GAP));
        const x = cursor;
        cursor += raw;
        return { ...s, x, w, first: i === 0, last: isLast };
    });
    const barEnd = cursor;

    const hovered = hover === null ? null : laid[hover];
    const tip: TooltipState | null =
        !hovered || width === 0
            ? null
            : {
                  x: hovered.x + hovered.w / 2,
                  y: height,
                  below: true,
                  title: tooltipTitle ?? hovered.label,
                  rows: [
                      {
                          label: hovered.label,
                          value: hovered.valueLabel
                              ? `${format(hovered.value)} · ${hovered.valueLabel}`
                              : format(hovered.value),
                          color: hovered.color,
                      },
                  ],
              };

    return (
        <div ref={ref} className="relative w-full" style={{ minHeight: height }}>
            {width > 0 && (
                <svg
                    role="img"
                    aria-label={ariaLabel}
                    width="100%"
                    height={height}
                    viewBox={`0 0 ${width} ${height}`}
                    onPointerLeave={() => setHover(null)}
                >
                    {laid.length === 0 ? (
                        <>
                            <rect
                                x={0}
                                y={barY}
                                width={Math.max(1, plotW)}
                                height={barHeight}
                                rx={4}
                                fill="var(--hover)"
                            />
                            <text
                                x={10}
                                y={height / 2 + 4}
                                fontSize={11}
                                fill={VIZ.muted}
                            >
                                {emptyMessage}
                            </text>
                        </>
                    ) : (
                        laid.map((s, i) => {
                            const candidates = s.valueLabel
                                ? [`${s.label} ${s.valueLabel}`, s.valueLabel]
                                : [s.label];
                            const fits = candidates.find(
                                (c) => s.w >= minLabelWidth && s.w >= estTextWidth(c, 11) + 14
                            );

                            return (
                                <g key={s.key}>
                                    <path
                                        d={roundedRect(s.x, barY, s.w, barHeight, 4, {
                                            tl: s.first && capStart,
                                            bl: s.first && capStart,
                                            tr: s.last,
                                            br: s.last,
                                        })}
                                        fill={s.color}
                                        opacity={hover === null || hover === i ? 1 : 0.72}
                                    />
                                    {inlineLabels && fits && (
                                        <text
                                            x={s.x + s.w / 2}
                                            y={height / 2 + 4}
                                            textAnchor="middle"
                                            fontSize={11}
                                            fontWeight={600}
                                            fill={s.ink}
                                        >
                                            {fits}
                                        </text>
                                    )}
                                    <rect
                                        x={s.x}
                                        y={0}
                                        width={Math.max(1, s.w + GAP)}
                                        height={height}
                                        fill="transparent"
                                        onPointerEnter={() => setHover(i)}
                                    />
                                </g>
                            );
                        })
                    )}

                    {(endLabel || endHint) && (
                        <g>
                            {endLabel && (
                                <text x={barEnd + 10} y={height / 2 + 4} fontSize={11} fontWeight={600} fill={VIZ.ink}>
                                    {endLabel}
                                </text>
                            )}
                            {endHint && (
                                <text
                                    x={barEnd + 10 + (endLabel ? estTextWidth(endLabel, 11) + 6 : 0)}
                                    y={height / 2 + 4}
                                    fontSize={11}
                                    fill={VIZ.muted}
                                >
                                    {endHint}
                                </text>
                            )}
                        </g>
                    )}
                </svg>
            )}
            <ChartTooltip tip={tip} width={width} />
        </div>
    );
};
