"use client";

import React, { useMemo, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import useSWR from "swr";
import { useAuth } from "@/ui/auth/AuthProvider";

/* =========================================================
   Types (match your current backend payload)
========================================================= */

type DecisionType =
  | "approve"
  | "reject"
  | "edit_confirm"
  | "manual_override"
  | "system_approve"
  | string;

type DecisionBy = "human" | "system" | "policy" | string;
type TriggerBy = "system" | "human" | "policy" | string;

type DiffValue = { before: string | null; after: string | null };

type AtlasRequestRow = {
  request_id: number;
  shop_code: string;

  item: { id: number; name: string };

  // timeline
  submitted_at: string | null;
  analyzed_at: string;
  decided_at: string | null;

  analysis_version: string;
  request_status: string;

  trigger: {
    by: TriggerBy | null;
    reason: string | null;
    replay: { original_request_id: number; index: number } | null;
  };

  error: { code: string | null; message: string | null } | null;

  before: {
    brand: string | null;
    condition: string | null; // "" may come
    color: string | null;
  };

  ai: {
    brand: string | null;
    condition: string | null;
    color: string | null;
    max_confidence: number | null;
    source: string | null;
    confidence_map: Record<string, number> | null;
  };

  decision: {
    type: DecisionType;
    by: DecisionBy;
    decided_at: string | null;
    user: { id: number; name: string } | null;
  } | null;

  diff: {
    brand: DiffValue;
    condition: DiffValue;
    color: DiffValue;
  } | null;

  final: {
    brand: string | null;
    condition?: string | null;
    color?: string | null;
    source: string | null;
    max_confidence?: number | null;
  } | null;
};

type ApiResponse = { requests: AtlasRequestRow[] };

/* =========================================================
   apiClient fetcher (no credentials include)
========================================================= */

function normalizeApiPath(path: string): string {
  // apiClient が /api prefix を付ける想定があるので、SWRキーが /api/... でも剥がして叩く
  return path.startsWith("/api/") ? path.replace(/^\/api/, "") : path;
}

function unwrap<T>(r: any): T {
  // axios-like {data} / fetch-like plain
  if (r && typeof r === "object" && "data" in r) return r.data as T;
  return r as T;
}

/* =========================================================
   UI utils
========================================================= */

function clsx(...parts: Array<string | false | null | undefined>) {
  return parts.filter(Boolean).join(" ");
}

function safe(v: any) {
  if (v === null || v === undefined) return "-";
  const s = String(v);
  return s.length ? s : "-";
}

function fmtDT(s?: string | null) {
  if (!s) return "-";
  return String(s).replace("T", " ").replace(".000000Z", "Z");
}

function pct(v?: number | null) {
  if (v === null || v === undefined) return null;
  return Math.round(v * 100);
}

function confTone(v?: number | null) {
  if (v === null || v === undefined)
    return "bg-gray-100 text-gray-700 ring-gray-200";
  if (v >= 0.85) return "bg-red-50 text-red-700 ring-red-200";
  if (v >= 0.7) return "bg-yellow-50 text-yellow-700 ring-yellow-200";
  return "bg-green-50 text-green-700 ring-green-200";
}

function ConfidencePill({ v }: { v?: number | null }) {
  if (v === null || v === undefined)
    return <span className="text-gray-400">-</span>;
  return (
    <span
      className={clsx(
        "inline-flex items-center px-2 py-0.5 rounded text-xs font-semibold ring-1",
        confTone(v),
      )}
    >
      {pct(v)}%
    </span>
  );
}

function StatusPill({
  status,
  error,
}: {
  status: string;
  error?: AtlasRequestRow["error"];
}) {
  const s = String(status ?? "").toLowerCase();
  if (error?.code || error?.message || s.includes("fail")) {
    return (
      <span className="px-2 py-0.5 rounded text-xs font-semibold bg-red-50 text-red-700 ring-1 ring-red-200">
        Failed
      </span>
    );
  }
  if (s.includes("pending") || s.includes("queue")) {
    return (
      <span className="px-2 py-0.5 rounded text-xs font-semibold bg-yellow-50 text-yellow-700 ring-1 ring-yellow-200">
        Pending
      </span>
    );
  }
  if (s.includes("done") || s.includes("active") || s.includes("complete")) {
    return (
      <span className="px-2 py-0.5 rounded text-xs font-semibold bg-green-50 text-green-700 ring-1 ring-green-200">
        Done
      </span>
    );
  }
  return (
    <span className="px-2 py-0.5 rounded text-xs font-semibold bg-gray-50 text-gray-700 ring-1 ring-gray-200">
      {safe(status)}
    </span>
  );
}

function DecisionBadge({
  decision,
}: {
  decision: AtlasRequestRow["decision"];
}) {
  const t = decision?.type ?? null;
  if (!t)
    return (
      <span className="px-2 py-0.5 rounded text-xs font-semibold bg-blue-50 text-blue-700 ring-1 ring-blue-200">
        Review
      </span>
    );
  if (t === "system_approve")
    return (
      <span className="px-2 py-0.5 rounded text-xs font-semibold bg-green-50 text-green-700 ring-1 ring-green-200">
        Auto
      </span>
    );
  if (t === "reject")
    return (
      <span className="px-2 py-0.5 rounded text-xs font-semibold bg-red-50 text-red-700 ring-1 ring-red-200">
        Rejected
      </span>
    );
  return (
    <span className="px-2 py-0.5 rounded text-xs font-semibold bg-yellow-50 text-yellow-700 ring-1 ring-yellow-200">
      Human
    </span>
  );
}

function Chip({
  tone,
  children,
}: {
  tone: "gray" | "blue" | "yellow" | "red" | "green";
  children: React.ReactNode;
}) {
  const cls =
    tone === "red"
      ? "bg-red-50 text-red-700 ring-red-200"
      : tone === "yellow"
        ? "bg-yellow-50 text-yellow-700 ring-yellow-200"
        : tone === "green"
          ? "bg-green-50 text-green-700 ring-green-200"
          : tone === "blue"
            ? "bg-blue-50 text-blue-700 ring-blue-200"
            : "bg-gray-50 text-gray-700 ring-gray-200";

  return (
    <span
      className={clsx(
        "inline-flex items-center px-2 py-0.5 rounded text-xs font-semibold ring-1",
        cls,
      )}
    >
      {children}
    </span>
  );
}

function EmptyHint({ text }: { text: string }) {
  return <div className="text-sm text-gray-500">{text}</div>;
}

/* =========================================================
  Tabs / filters
========================================================= */
type TabKey = "timeline" | "diff" | "confidence" | "replay" | "policy";

function TabHeader({
  active,
  onChange,
  counts,
}: {
  active: TabKey;
  onChange: (k: TabKey) => void;
  counts: {
    diff: number;
    hasConfidenceMap: number;
    hasReplay: number;
    hasPolicy: number;
  };
}) {
  const tabs: Array<{
    key: TabKey;
    label: string;
    hint: string;
    badge?: number;
  }> = [
    { key: "timeline", label: "Timeline", hint: "Submitted→Analyzed→Decided" },
    {
      key: "diff",
      label: "Diff",
      hint: "Before/After 強調",
      badge: counts.diff,
    },
    {
      key: "confidence",
      label: "Confidence",
      hint: "confidence_map",
      badge: counts.hasConfidenceMap,
    },
    { key: "replay", label: "Replay", hint: "replay", badge: counts.hasReplay },
    {
      key: "policy",
      label: "Policy",
      hint: "auto rationale",
      badge: counts.hasPolicy,
    },
  ];

  return (
    <div className="flex flex-wrap items-center gap-2 border-b pb-3">
      {tabs.map((t) => (
        <button
          key={t.key}
          type="button"
          onClick={() => onChange(t.key)}
          className={clsx(
            "px-3 py-1.5 rounded-md text-sm font-semibold ring-1 transition",
            active === t.key
              ? "bg-gray-900 text-white ring-gray-900"
              : "bg-white text-gray-800 ring-gray-200 hover:bg-gray-50",
          )}
        >
          <span className="inline-flex items-center gap-2">
            {t.label}
            {!!t.badge && t.badge > 0 && (
              <span
                className={clsx(
                  "text-xs px-1.5 py-0.5 rounded",
                  active === t.key
                    ? "bg-white/20 text-white"
                    : "bg-gray-100 text-gray-700",
                )}
              >
                {t.badge}
              </span>
            )}
          </span>
          <span
            className={clsx(
              "ml-2 text-xs font-normal",
              active === t.key ? "text-white/80" : "text-gray-500",
            )}
          >
            {t.hint}
          </span>
        </button>
      ))}
    </div>
  );
}

type FilterKey =
  | "all"
  | "review"
  | "high_risk_review"
  | "human"
  | "auto"
  | "rejected"
  | "failed";

type SortKey = "analyzed_desc" | "confidence_desc" | "decided_desc";

function isFailed(r: AtlasRequestRow) {
  return (
    !!r.error?.code ||
    !!r.error?.message ||
    String(r.request_status).toLowerCase().includes("fail")
  );
}
function isReview(r: AtlasRequestRow) {
  return r.decision === null;
}
function isAuto(r: AtlasRequestRow) {
  return r.decision?.type === "system_approve";
}
function isRejected(r: AtlasRequestRow) {
  return r.decision?.type === "reject";
}
function isHuman(r: AtlasRequestRow) {
  return (
    !!r.decision &&
    r.decision.type !== "system_approve" &&
    r.decision.type !== "reject"
  );
}
function highRiskReview(r: AtlasRequestRow) {
  const c = r.ai?.max_confidence;
  return isReview(r) && c !== null && c !== undefined && c >= 0.85;
}

function riskSignals(
  r: AtlasRequestRow,
): Array<{ tone: "red" | "yellow" | "blue"; text: string }> {
  const out: Array<{ tone: "red" | "yellow" | "blue"; text: string }> = [];
  const c = r.ai?.max_confidence;
  const decided = !!r.decision;

  if (highRiskReview(r))
    out.push({ tone: "red", text: "HighConfidenceReview" });

  if (
    decided &&
    (r.decision?.type === "approve" || r.decision?.type === "system_approve") &&
    c != null &&
    c < 0.7
  ) {
    out.push({ tone: "red", text: "LowConfidenceApproved" });
  }

  if (decided && !r.final)
    out.push({ tone: "yellow", text: "FinalMissingAfterDecision" });

  if (
    decided &&
    !r.diff &&
    (r.decision?.type === "approve" ||
      r.decision?.type === "edit_confirm" ||
      r.decision?.type === "manual_override" ||
      r.decision?.type === "system_approve")
  ) {
    out.push({ tone: "yellow", text: "DiffMissingAfterDecision" });
  }

  if (decided && r.decision?.type === "approve" && !r.final && r.ai?.brand) {
    out.push({ tone: "blue", text: "BrandUnregisteredCandidate" });
  }

  return out;
}

function compareByDateDesc(a?: string | null, b?: string | null) {
  const ta = a ? Date.parse(a.replace(" ", "T")) : 0;
  const tb = b ? Date.parse(b.replace(" ", "T")) : 0;
  return tb - ta;
}

/* =========================================================
   Confidence mini bars
========================================================= */

function MiniConfidenceBars({ map }: { map: Record<string, number> | null }) {
  if (!map) return <span className="text-xs text-gray-400">no map</span>;

  const entries = Object.entries(map)
    .filter(([, v]) => typeof v === "number")
    .sort(
      ([a], [b]) =>
        ["brand", "condition", "color"].indexOf(a) -
        ["brand", "condition", "color"].indexOf(b),
    );

  if (!entries.length)
    return <span className="text-xs text-gray-400">no map</span>;

  const tone = (v: number) =>
    v >= 0.85 ? "bg-red-200" : v >= 0.7 ? "bg-yellow-200" : "bg-green-200";

  return (
    <div className="space-y-1">
      {entries.map(([k, v]) => {
        const p = Math.round(v * 100);
        return (
          <div key={k} className="flex items-center gap-2">
            <div className="w-20 text-[11px] text-gray-500">{k}</div>
            <div className="flex-1 h-2 rounded bg-gray-200 overflow-hidden">
              <div
                className={clsx("h-2", tone(v))}
                style={{ width: `${p}%` }}
              />
            </div>
            <div className="w-10 text-[11px] font-semibold text-gray-700 text-right">
              {p}%
            </div>
          </div>
        );
      })}
    </div>
  );
}

/* =========================================================
   Reason labels
========================================================= */

function beforeValueLabel(v: string | null) {
  if (v === null) return { text: "—", hint: "N/A (no field / not captured)" };
  if (v === "") return { text: "(empty)", hint: "Empty input" };
  return { text: v, hint: null };
}

function finalMissingReason(r: AtlasRequestRow): string {
  if (!r.decision) return "No decision yet";
  if (r.decision.type === "approve" && r.ai?.brand)
    return "Approved but SoT not applied (brand may be unregistered)";
  return "Decided but SoT not applied yet";
}

function diffMissingReason(r: AtlasRequestRow): string {
  if (!r.decision) return "No decision yet";
  if (r.decision.type === "manual_override")
    return "Override decided; snapshot may not have been saved for diff";
  return "Snapshot missing or diff not computed";
}

/* =========================================================
   Views
========================================================= */

function SectionTitle({
  title,
  subtitle,
}: {
  title: string;
  subtitle?: string;
}) {
  return (
    <div>
      <div className="text-sm font-semibold text-gray-900">{title}</div>
      {subtitle && <div className="text-xs text-gray-500">{subtitle}</div>}
    </div>
  );
}

function RowHeader({ r }: { r: AtlasRequestRow }) {
  const signals = riskSignals(r);

  return (
    <div className="flex flex-col gap-2 md:flex-row md:items-start md:justify-between">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <div className="text-base font-semibold text-gray-900">
            商品名：{safe(r.item?.name)}{" "}
            <span className="text-gray-500">（Item#{safe(r.item?.id)}）</span>
          </div>

          <StatusPill status={r.request_status} error={r.error} />
          <DecisionBadge decision={r.decision} />
          <ConfidencePill v={r.ai?.max_confidence} />

          {signals.map((s, idx) => (
            <Chip key={`${s.text}-${idx}`} tone={s.tone}>
              {s.text}
            </Chip>
          ))}
        </div>

        <div className="mt-1 text-xs text-gray-500">
          Shop名:{" "}
          <span className="font-semibold text-gray-700">{r.shop_code}</span> /
          Analyzer:{" "}
          <span className="font-semibold text-gray-700">
            {r.analysis_version}
          </span>{" "}
          / Request#{r.request_id}
        </div>

        {r.error?.message && (
          <div className="mt-2 rounded-md bg-red-50 p-2 text-xs text-red-700 ring-1 ring-red-200">
            Error: {safe(r.error.message)}
          </div>
        )}
      </div>

      {/* ✅ 導線を強化（機能追加、デザインは現状テイストで最小） */}
      <div className="flex flex-wrap items-center gap-3 text-xs">
        {r.decision ? (
          <Link
            href={`/shops/${r.shop_code}/dashboard/atlas/history/${r.request_id}`}
            className="font-semibold text-gray-900 underline"
          >
            履歴
          </Link>
        ) : (
          <Link
            href={`/shops/${r.shop_code}/dashboard/atlas/review/${r.request_id}`}
            className="font-semibold text-blue-600 underline"
          >
            判断
          </Link>
        )}

        <Link
          href={`/shops/${r.shop_code}/dashboard/atlas/compare/${r.request_id}`}
          className="font-semibold text-gray-900 underline"
        >
          Compare
        </Link>

        <Link
          href={`/shops/${r.shop_code}/dashboard/atlas/decide/${r.request_id}`}
          className="font-semibold text-gray-900 underline"
        >
          Decide
        </Link>

        <Link
          href={`/shops/${r.shop_code}/dashboard/atlas/requests/${r.request_id}`}
          className="font-semibold text-gray-900 underline"
        >
          Raw
        </Link>
      </div>
    </div>
  );
}

function Card({
  title,
  children,
}: {
  title: string;
  children: React.ReactNode;
}) {
  return (
    <div className="rounded-xl border bg-gray-50 p-3">
      <div className="text-sm font-semibold text-gray-900">{title}</div>
      <div className="mt-2 space-y-1">{children}</div>
    </div>
  );
}

function Line({
  label,
  value,
  hint,
}: {
  label: string;
  value: string;
  hint?: string | null;
}) {
  return (
    <div className="flex items-start justify-between gap-3 text-xs">
      <div className="text-gray-500">{label}</div>
      <div className="text-right">
        <div className="font-semibold text-gray-800">{value}</div>
        {hint ? <div className="text-[11px] text-gray-400">{hint}</div> : null}
      </div>
    </div>
  );
}

function TimelineView({ rows }: { rows: AtlasRequestRow[] }) {
  if (!rows.length)
    return <EmptyHint text="表示できるリクエストがありません。" />;

  return (
    <div className="space-y-4">
      <SectionTitle
        title="Timeline（時系列）"
        subtitle="Review Queue + Audit Timeline（最優先）"
      />

      <div className="space-y-3">
        {rows.map((r) => {
          const bBrand = beforeValueLabel(r.before?.brand ?? null);

          return (
            <div key={r.request_id} className="rounded-xl border bg-white p-4">
              <RowHeader r={r} />

              <div className="mt-4 grid gap-3 md:grid-cols-3">
                <div className="rounded-xl border bg-gray-50 p-3">
                  <div className="text-sm font-semibold">
                    Submitted（入力内容）
                  </div>
                  <div className="mt-2">
                    <Line
                      label="Pending AI Analysis"
                      value={bBrand.text}
                      hint={bBrand.hint}
                    />
                    <Line label="at" value={fmtDT(r.submitted_at)} />
                  </div>
                </div>

                <div className="rounded-xl border bg-gray-50 p-3">
                  <div className="text-sm font-semibold">
                    Analyzed (AI)（解析結果）
                  </div>
                  <div className="mt-2">
                    <Line label="Brand" value={safe(r.ai.brand)} />
                    <Line label="Condition" value={safe(r.ai.condition)} />
                    <Line label="Color" value={safe(r.ai.color)} />
                    <Line
                      label="Confidence"
                      value={
                        r.ai.max_confidence != null
                          ? `${pct(r.ai.max_confidence)}%`
                          : "-"
                      }
                    />
                    <Line label="Source" value={safe(r.ai.source)} />
                    <Line label="at" value={fmtDT(r.analyzed_at)} />
                  </div>

                  <div className="mt-3">
                    <div className="text-xs font-semibold text-gray-900">
                      Confidence Map
                    </div>
                    <div className="mt-2">
                      <MiniConfidenceBars map={r.ai.confidence_map} />
                    </div>
                  </div>
                </div>

                <div className="rounded-xl border bg-gray-50 p-3">
                  <div className="text-sm font-semibold">Decided / Final</div>
                  <div className="mt-2">
                    <Line label="Decision" value={safe(r.decision?.type)} />
                    <Line label="By" value={safe(r.decision?.by)} />
                    <Line
                      label="User"
                      value={
                        r.decision?.user
                          ? `${r.decision.user.name} (User#${r.decision.user.id})`
                          : "-"
                      }
                    />
                    <Line
                      label="Decided at"
                      value={fmtDT(r.decided_at ?? r.decision?.decided_at)}
                    />
                    {r.final?.brand ? (
                      <>
                        <Line label="Final Brand" value={safe(r.final.brand)} />
                        <Line
                          label="Final Condition"
                          value={safe(r.final.condition)}
                        />
                        <Line label="Final Color" value={safe(r.final.color)} />
                        <Line
                          label="Final Confidence"
                          value={
                            r.final.max_confidence != null
                              ? `${pct(r.final.max_confidence)}%`
                              : "-"
                          }
                        />
                        <Line
                          label="Final Source"
                          value={safe(r.final.source)}
                        />
                      </>
                    ) : (
                      <Line
                        label="Final"
                        value="(missing)"
                        hint={finalMissingReason(r)}
                      />
                    )}
                  </div>
                </div>
              </div>

              <div className="mt-3 text-xs text-gray-500">
                Diff: {r.diff ? "available" : diffMissingReason(r)} / SoT:{" "}
                {r.final ? "applied" : finalMissingReason(r)}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}

function DiffView({ rows }: { rows: AtlasRequestRow[] }) {
  const diffRows = useMemo(() => rows.filter((r) => !!r.diff), [rows]);

  return (
    <div className="space-y-4">
      <SectionTitle
        title="Diff（入力→解析→判断）"
        subtitle="Before/After を強調（教育・監査・品質）"
      />
      {diffRows.length === 0 && (
        <div className="rounded-xl border bg-white p-4">
          <EmptyHint text="Diff が存在するリクエストがありません。" />
        </div>
      )}

      <div className="space-y-3">
        {rows.map((r) => (
          <div key={r.request_id} className="rounded-xl border bg-white p-4">
            <RowHeader r={r} />
            <div className="mt-4 grid gap-3 md:grid-cols-3">
              <Card title="Before (Draft)">
                <Line
                  label="Brand"
                  value={beforeValueLabel(r.before?.brand ?? null).text}
                  hint={beforeValueLabel(r.before?.brand ?? null).hint}
                />
                <Line
                  label="Condition"
                  value={beforeValueLabel(r.before?.condition ?? null).text}
                  hint={beforeValueLabel(r.before?.condition ?? null).hint}
                />
                <Line
                  label="Color"
                  value={beforeValueLabel(r.before?.color ?? null).text}
                  hint={beforeValueLabel(r.before?.color ?? null).hint}
                />
              </Card>

              <Card title="After (Diff)">
                {r.diff ? (
                  (["brand", "condition", "color"] as const).map((k) => {
                    const d = r.diff?.[k] ?? { before: null, after: null };
                    const changed = d.before !== d.after;

                    return (
                      <div key={k} className="rounded-md border bg-white p-2">
                        <div className="flex items-center justify-between">
                          <div className="text-gray-500 text-xs">{k}</div>
                          <Chip tone={changed ? "yellow" : "gray"}>
                            {changed ? "changed" : "same"}
                          </Chip>
                        </div>

                        <div className="mt-1 text-xs">
                          <span
                            className={clsx(
                              "inline-block px-2 py-0.5 rounded mr-2",
                              changed
                                ? "bg-red-50 text-red-700"
                                : "bg-gray-50 text-gray-600",
                            )}
                          >
                            {safe(d.before)}
                          </span>
                          <span className="text-gray-400">→</span>
                          <span
                            className={clsx(
                              "inline-block px-2 py-0.5 rounded ml-2",
                              changed
                                ? "bg-green-50 text-green-700"
                                : "bg-gray-50 text-gray-600",
                            )}
                          >
                            {safe(d.after)}
                          </span>
                        </div>
                      </div>
                    );
                  })
                ) : (
                  <div className="text-xs text-gray-500">
                    {diffMissingReason(r)}
                  </div>
                )}
              </Card>

              <Card title="Final (SoT)">
                {r.final?.brand ? (
                  <>
                    <Line label="Brand" value={safe(r.final.brand)} />
                    <Line label="Source" value={safe(r.final.source)} />
                  </>
                ) : (
                  <Line
                    label="SoT"
                    value="(missing)"
                    hint={finalMissingReason(r)}
                  />
                )}

                <div className="mt-2 text-xs text-gray-500">AI</div>
                <Line label="Brand" value={safe(r.ai.brand)} />
                <Line label="Condition" value={safe(r.ai.condition)} />
                <Line label="Color" value={safe(r.ai.color)} />
              </Card>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

function ConfidenceView({ rows }: { rows: AtlasRequestRow[] }) {
  const withMap = useMemo(
    () =>
      rows.filter(
        (r) =>
          r.ai?.confidence_map &&
          Object.keys(r.ai.confidence_map ?? {}).length > 0,
      ),
    [rows],
  );

  return (
    <div className="space-y-4">
      <SectionTitle
        title="Confidence（解析結果一致率）"
        subtitle="confidence_map を可視化（研究・改善の基盤）"
      />
      {withMap.length === 0 && (
        <div className="rounded-xl border bg-white p-4">
          <EmptyHint text="confidence_map が存在する行がありません。" />
        </div>
      )}

      <div className="space-y-3">
        {withMap.map((r) => (
          <div key={r.request_id} className="rounded-xl border bg-white p-4">
            <RowHeader r={r} />
            <div className="mt-4 grid gap-3 md:grid-cols-2">
              <Card title="AI Result">
                <Line label="Brand" value={safe(r.ai.brand)} />
                <Line label="Condition" value={safe(r.ai.condition)} />
                <Line label="Color" value={safe(r.ai.color)} />
                <Line label="Source" value={safe(r.ai.source)} />
                <Line
                  label="Max"
                  value={
                    r.ai.max_confidence != null
                      ? `${pct(r.ai.max_confidence)}%`
                      : "-"
                  }
                />
              </Card>

              <Card title="Confidence Map">
                <MiniConfidenceBars map={r.ai.confidence_map} />
              </Card>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

function ReplayView({ rows }: { rows: AtlasRequestRow[] }) {
  const candidates = useMemo(
    () => rows.filter((r) => !!r.trigger?.replay),
    [rows],
  );

  return (
    <div className="space-y-4">
      <SectionTitle title="Replay" subtitle="Replay 情報（再解析系譜）" />

      {candidates.length === 0 && (
        <div className="rounded-xl border bg-white p-4">
          <EmptyHint text="Replay 情報がありません（trigger.replay が null）。" />
        </div>
      )}

      <div className="space-y-3">
        {candidates.map((r) => (
          <div key={r.request_id} className="rounded-xl border bg-white p-4">
            <RowHeader r={r} />
            <div className="mt-3 rounded-xl border bg-gray-50 p-3 text-xs text-gray-700">
              Replay:{" "}
              <span className="font-semibold">#{r.trigger.replay!.index}</span>{" "}
              from{" "}
              <span className="font-semibold">
                #{r.trigger.replay!.original_request_id}
              </span>
              <div className="mt-1 text-gray-500">
                reason: {safe(r.trigger.reason)}
              </div>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

function PolicyView({ rows }: { rows: AtlasRequestRow[] }) {
  const candidates = useMemo(
    () => rows.filter((r) => r.decision?.type === "system_approve"),
    [rows],
  );

  return (
    <div className="space-y-4">
      <SectionTitle
        title="Policy"
        subtitle="Auto 判定（system_approve）の根拠（将来拡張）"
      />

      {candidates.length === 0 && (
        <div className="rounded-xl border bg-white p-4">
          <EmptyHint text="system_approve がまだありません。DecisionPolicy 実装後にここが“説明可能なAuto”になります。" />
        </div>
      )}

      <div className="space-y-3">
        {candidates.map((r) => (
          <div key={r.request_id} className="rounded-xl border bg-white p-4">
            <RowHeader r={r} />
            <div className="mt-3 rounded-xl border bg-gray-50 p-3 text-xs text-gray-700">
              <div>
                Decision:{" "}
                <span className="font-semibold">{safe(r.decision?.type)}</span>
              </div>
              <div>
                Confidence:{" "}
                <span className="font-semibold">
                  {r.ai?.max_confidence != null
                    ? `${pct(r.ai.max_confidence)}%`
                    : "-"}
                </span>
              </div>
              <div className="text-gray-500 mt-2">
                （将来）policy.outcome / explanation / factors を返して “Auto
                rationale” を表示します。
              </div>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

/* =========================================================
   Main Page
========================================================= */

export default function AtlasRequestsPage() {
  const params = useParams();
  const shop_code = String((params as any)?.shop_code ?? "");

  const { authReady, isAuthenticated, user, apiClient } = useAuth() as any;

  const isReviewer =
    user?.shop_roles?.some(
      (r: any) =>
        r.shop_code === shop_code && ["owner", "manager"].includes(r.role),
    ) ?? false;

  // ✅ apiClient 版 fetcher（JWT/IdaaSでも動く）
  const apiFetcher = async (url: string): Promise<ApiResponse> => {
    const r = await apiClient.get(normalizeApiPath(url));
    return unwrap<ApiResponse>(r);
  };

  const { data, error, isLoading } = useSWR<ApiResponse>(
    isReviewer ? `/api/shops/${shop_code}/atlas/requests` : null,
    apiFetcher,
  );

  const all = data?.requests ?? [];

  // filter + sort
  const [filter, setFilter] = useState<FilterKey>("review");
  const [sort, setSort] = useState<SortKey>("confidence_desc");

  const filtered = useMemo(() => {
    let rows = [...all];

    rows = rows.filter((r) => {
      switch (filter) {
        case "all":
          return true;
        case "review":
          return isReview(r);
        case "high_risk_review":
          return highRiskReview(r);
        case "human":
          return isHuman(r);
        case "auto":
          return isAuto(r);
        case "rejected":
          return isRejected(r);
        case "failed":
          return isFailed(r);
        default:
          return true;
      }
    });

    rows.sort((a, b) => {
      if (sort === "confidence_desc") {
        const ca = a.ai?.max_confidence ?? -1;
        const cb = b.ai?.max_confidence ?? -1;
        return cb - ca;
      }
      if (sort === "decided_desc") {
        return compareByDateDesc(
          a.decided_at ?? a.decision?.decided_at ?? null,
          b.decided_at ?? b.decision?.decided_at ?? null,
        );
      }
      return compareByDateDesc(a.analyzed_at, b.analyzed_at);
    });

    return rows;
  }, [all, filter, sort]);

  const counts = useMemo(() => {
    const diff = filtered.filter((r) => !!r.diff).length;
    const hasConfidenceMap = filtered.filter(
      (r) =>
        !!r.ai?.confidence_map &&
        Object.keys(r.ai.confidence_map ?? {}).length > 0,
    ).length;
    const hasReplay = filtered.filter((r) => !!r.trigger?.replay).length;
    const hasPolicy = filtered.filter(
      (r) => r.decision?.type === "system_approve",
    ).length;
    return { diff, hasConfidenceMap, hasReplay, hasPolicy };
  }, [filtered]);

  const [activeTab, setActiveTab] = useState<TabKey>("timeline");

  if (!authReady || !isAuthenticated)
    return <div className="p-6">認証確認中...</div>;
  if (!isReviewer) return <div className="p-6">アクセス権限がありません。</div>;
  if (isLoading) return <div className="p-6">読み込み中...</div>;
  if (error)
    return (
      <div className="p-6 text-red-600">
        取得失敗：{(error as Error).message}
      </div>
    );

  return (
    <div className="p-6 space-y-4">
      <div className="flex flex-col gap-2 md:flex-row md:items-end md:justify-between">
        <div>
          <h1 className="text-2xl font-semibold text-gray-900">
            Atlas Console（Requests）
          </h1>
          <div className="text-sm text-gray-500 mt-1">
            Shop:{" "}
            <span className="font-semibold text-gray-800">{shop_code}</span> /
            Total:{" "}
            <span className="font-semibold text-gray-800">{all.length}</span> /
            Showing:{" "}
            <span className="font-semibold text-gray-800">
              {filtered.length}
            </span>
          </div>
        </div>

        {/* ✅ 導線：戻る/履歴一覧 */}
        <div className="flex items-center gap-3 flex-wrap">
          <Link
            href={`/shops/${shop_code}/dashboard`}
            className="text-sm font-semibold text-gray-900 underline"
          >
            ダッシュボードへ戻る
          </Link>

          <Link
            href={`/shops/${shop_code}/dashboard/atlas/history`}
            className="text-sm font-semibold text-gray-900 underline"
          >
            判断履歴一覧
          </Link>
        </div>
      </div>

      {/* Filter / Sort controls */}
      <div className="rounded-xl border bg-white p-3 flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-xs font-semibold text-gray-700">Filter</span>
          {(
            [
              ["review", "解析履歴(判断待ち)"],
              ["human", "管理者判断履歴"],
              ["all", "全て"],
              ["rejected", "解析結果却下履歴"],
              ["auto", "自動化履歴"],
              ["high_risk_review", "解析（高リスク）"],
              ["failed", "失敗履歴"],
            ] as Array<[FilterKey, string]>
          ).map(([k, label]) => (
            <button
              key={k}
              type="button"
              onClick={() => setFilter(k)}
              className={clsx(
                "px-2 py-1 rounded text-xs font-semibold ring-1",
                filter === k
                  ? "bg-gray-900 text-white ring-gray-900"
                  : "bg-white text-gray-700 ring-gray-200 hover:bg-gray-50",
              )}
            >
              {label}
            </button>
          ))}
        </div>

        <div className="flex items-center gap-2">
          <span className="text-xs font-semibold text-gray-700">Sort</span>
          <select
            className="text-xs border rounded px-2 py-1"
            value={sort}
            onChange={(e) => setSort(e.target.value as SortKey)}
          >
            <option value="confidence_desc">Confidence ↓</option>
            <option value="analyzed_desc">Analyzed ↓</option>
            <option value="decided_desc">Decided ↓</option>
          </select>
        </div>
      </div>

      <TabHeader active={activeTab} onChange={setActiveTab} counts={counts} />

      <div className="rounded-2xl border bg-white p-4">
        {activeTab === "timeline" && <TimelineView rows={filtered} />}
        {activeTab === "diff" && <DiffView rows={filtered} />}
        {activeTab === "confidence" && <ConfidenceView rows={filtered} />}
        {activeTab === "replay" && <ReplayView rows={filtered} />}
        {activeTab === "policy" && <PolicyView rows={filtered} />}
      </div>

      {filtered.length === 0 && (
        <div className="text-sm text-gray-500">
          該当するリクエストがありません。
        </div>
      )}

      <div className="rounded-xl border bg-gray-50 p-4 text-sm text-gray-700">
        このページは AtlasKernel v3 の「判断基盤コンソール」です。
        <br />
        あなたが指定した優先順（Review Queue → Risk → Mini Confidence → Decision
        詳細 → 理由）で強化されています。
      </div>
    </div>
  );
}
