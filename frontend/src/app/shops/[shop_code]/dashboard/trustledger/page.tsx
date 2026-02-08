"use client";

import Link from "next/link";
import { useMemo, useEffect, useState, useCallback, Fragment } from "react";
import { useParams, useRouter } from "next/navigation";
import { useAuth } from "@/ui/auth/AuthProvider";
import styles from "./TrustLedgerDashboard.module.css";

type PresetKey = "today" | "w1" | "w2" | "m1" | "m3" | "m6" | "y1" | "custom";
type PostingTypeFilter = "all" | "sale" | "fee" | "refund";

type LedgerSummary = {
  shop_id: number;
  from: string;
  to: string;
  currency: string;
  sales_total: number;
  refund_total: number;
  net_total: number;
  postings_count: number;
};

type LedgerEntryLine = {
  account_code: string;
  side: "debit" | "credit";
  amount: number;
  currency?: string;
};

type LedgerEntryItem = {
  posting_id: number;
  occurred_at: string;
  posting_type: "sale" | "fee" | "refund" | string;
  order_id: number | null;
  payment_id: number | null;
  source_provider: string;
  source_event_id: string;
  currency: string;
  entries: LedgerEntryLine[];
  amount?: number;
};

type LedgerEntriesResponse = {
  items: LedgerEntryItem[];
  next_cursor: string | null;
};

type ReconciliationItem = {
  payment_id: number;
  order_id: number;
  shop_id: number;
  provider_payment_id: string;
  amount: number;
  currency: string;
  method: string;
  updated_at: string;
};

type ReconciliationResponse = {
  shop_id: number;
  from: string;
  to: string;
  missing_sale_count: number;
  missing_sales: ReconciliationItem[];
};

type Balance = {
  account_id: number;
  available_amount: number;
  pending_amount: number;
  held_amount: number;
  currency: string;
  calculated_at: string;
};

function pad2(n: number) {
  return String(n).padStart(2, "0");
}
function formatDateYYYYMMDD(d: Date) {
  return `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}`;
}
function addDays(date: Date, days: number) {
  const d = new Date(date);
  d.setDate(d.getDate() + days);
  return d;
}

function money(n: number | null | undefined) {
  if (typeof n !== "number") return "-";
  return n.toLocaleString();
}

function postingAmount(it: LedgerEntryItem): number {
  if (typeof it.amount === "number") return it.amount;
  return (it.entries ?? [])
    .filter((l) => l.side === "debit")
    .reduce((acc, l) => acc + (typeof l.amount === "number" ? l.amount : 0), 0);
}

function sumPostingAmount(
  items: LedgerEntryItem[],
  postingType: string,
): number {
  return items
    .filter((x) => x.posting_type === postingType)
    .reduce((acc, x) => acc + postingAmount(x), 0);
}

/**
 * ✅ ここが本質の修正点：
 * - 以前：fetch + credentials: include（Cookie前提）
 * - 今回：useAuth().apiClient（SanctumでもJWTでも同じ呼び方で動く）
 *
 * ★機能/デザイン/ロジックは一切変えない
 */
function normalizeApiPath(path: string): string {
  // apiClient は内部で /api prefix を付ける設計が多いので、ここで /api を剥がして統一
  if (path.startsWith("/api/")) return path.replace(/^\/api/, "");
  return path;
}

export default function TrustLedgerDashboardPage() {
  const { shop_code } = useParams<{ shop_code: string }>();
  const router = useRouter();

  const {
    user,
    isAuthenticated,
    isLoading: isAuthLoading,
    authReady,
    apiClient,
  } = useAuth() as any;

  // --- Auth gate ---
  const roleInShop = useMemo(() => {
    const r = user?.shop_roles?.find((x: any) => x.shop_code === shop_code);
    return r?.role ?? null;
  }, [user, shop_code]);

  const isShopStaff = useMemo(() => {
    return ["owner", "manager", "staff"].includes(roleInShop ?? "");
  }, [roleInShop]);

  const canOperateReconciliation = useMemo(() => {
    return ["owner", "manager"].includes(roleInShop ?? "");
  }, [roleInShop]);

  const shopId = useMemo<number | null>(() => {
    const r = user?.shop_roles?.find((x: any) => x.shop_code === shop_code);
    return typeof r?.shop_id === "number" ? r.shop_id : null;
  }, [user, shop_code]);

  // --- Period preset ---
  const [preset, setPreset] = useState<PresetKey>("w1");
  const [from, setFrom] = useState<string>(() =>
    formatDateYYYYMMDD(addDays(new Date(), -7)),
  );
  const [to, setTo] = useState<string>(() => formatDateYYYYMMDD(new Date()));

  useEffect(() => {
    const today = new Date();
    const end = formatDateYYYYMMDD(today);
    if (preset === "custom") return;

    const days =
      preset === "today"
        ? 0
        : preset === "w1"
          ? 7
          : preset === "w2"
            ? 14
            : preset === "m1"
              ? 30
              : preset === "m3"
                ? 90
                : preset === "m6"
                  ? 180
                  : 365;

    setFrom(formatDateYYYYMMDD(addDays(today, -days)));
    setTo(end);
  }, [preset]);

  // --- Filters ---
  const [postingType, setPostingType] = useState<PostingTypeFilter>("all");

  // --- Data states (all settled style) ---
  const [summary, setSummary] = useState<LedgerSummary | null>(null);
  const [entries, setEntries] = useState<LedgerEntryItem[]>([]);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [entriesKpi, setEntriesKpi] = useState<LedgerEntryItem[]>([]);
  const [recon, setRecon] = useState<ReconciliationResponse | null>(null);
  const [accountId, setAccountId] = useState<number | null>(null);
  const [balance, setBalance] = useState<Balance | null>(null);

  // UI
  const [expandedPostingIds, setExpandedPostingIds] = useState<
    Record<number, boolean>
  >({});
  const [busy, setBusy] = useState(false);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);
  const [toast, setToast] = useState<string | null>(null);

  // Hold/Payout
  const [holdAmount, setHoldAmount] = useState<number>(1000);
  const [holdReason, setHoldReason] = useState<string>("shipment_pending");
  const [lastHoldId, setLastHoldId] = useState<number | null>(null);

  const [payoutAmount, setPayoutAmount] = useState<number>(2000);
  const [lastPayoutId, setLastPayoutId] = useState<number | null>(null);

  const feeTotalApprox = useMemo(
    () => sumPostingAmount(entriesKpi, "fee"),
    [entriesKpi],
  );

  const netAfterFeeApprox = useMemo(() => {
    if (!summary) return null;
    return summary.net_total - feeTotalApprox;
  }, [summary, feeTotalApprox]);

  const filteredEntries = useMemo(() => {
    if (postingType === "all") return entries;
    return entries.filter((x) => x.posting_type === postingType);
  }, [entries, postingType]);

  const toggleExpand = useCallback((postingId: number) => {
    setExpandedPostingIds((prev) => ({
      ...prev,
      [postingId]: !prev[postingId],
    }));
  }, []);

  // ✅ apiClient 経由の GET/POST（SanctumでもJWTでもOK）
  const apiGet = useCallback(
    async <T,>(path: string): Promise<T> => {
      return (await apiClient.get(normalizeApiPath(path))) as T;
    },
    [apiClient],
  );

  const apiPostJson = useCallback(
    async <T,>(path: string, body?: any): Promise<T> => {
      return (await apiClient.post(normalizeApiPath(path), body ?? {})) as T;
    },
    [apiClient],
  );

  // --- Core loader ---
  const loadAll = useCallback(
    async (reset = true) => {
      if (!shopId) return;

      setBusy(true);
      setErrorMsg(null);

      try {
        const tasks = await Promise.allSettled([
          apiGet<LedgerSummary>(
            `/ledger/summary?shop_id=${shopId}&from=${from}&to=${to}`,
          ),
          apiGet<LedgerEntriesResponse>(
            `/ledger/entries?shop_id=${shopId}&from=${from}&to=${to}&limit=20`,
          ),
          apiGet<LedgerEntriesResponse>(
            `/ledger/entries?shop_id=${shopId}&from=${from}&to=${to}&limit=200`,
          ),
          apiGet<ReconciliationResponse>(
            `/ledger/reconciliation?shop_id=${shopId}&from=${from}&to=${to}&limit=50`,
          ),
          apiPostJson<{ ok: boolean; account_id: number }>(
            `/shops/${shopId}/balance/recalculate?from=${from}&to=${to}`,
            {},
          ),
        ]);

        const [s0, e0, ek0, r0, recalc0] = tasks;

        if (s0.status === "fulfilled") setSummary(s0.value);
        else setSummary(null);

        if (e0.status === "fulfilled") {
          setEntries(e0.value.items ?? []);
          setNextCursor(e0.value.next_cursor ?? null);
        } else {
          setEntries([]);
          setNextCursor(null);
        }

        if (ek0.status === "fulfilled") setEntriesKpi(ek0.value.items ?? []);
        else setEntriesKpi([]);

        if (r0.status === "fulfilled") setRecon(r0.value);
        else setRecon(null);

        if (recalc0.status === "fulfilled") {
          setAccountId(recalc0.value.account_id);

          const b = await apiGet<Balance>(
            `/accounts/${recalc0.value.account_id}/balance`,
          );
          setBalance(b);
        }

        if (reset) {
          setExpandedPostingIds({});
          setLastHoldId(null);
          setLastPayoutId(null);
        }

        const errs = tasks.filter(
          (t) => t.status === "rejected",
        ) as PromiseRejectedResult[];

        if (errs.length > 0) {
          setErrorMsg(
            errs.map((e) => e.reason?.message ?? String(e.reason)).join("\n"),
          );
        }
      } catch (e: any) {
        setErrorMsg(e?.message ?? String(e));
      } finally {
        setBusy(false);
      }
    },
    [shopId, from, to, apiGet, apiPostJson],
  );

  useEffect(() => {
    if (!shopId) return;
    loadAll(true);
  }, [shopId, from, to, loadAll]);

  const loadMore = useCallback(async () => {
    if (!shopId || !nextCursor) return;

    setBusy(true);
    setErrorMsg(null);
    try {
      const e = await apiGet<LedgerEntriesResponse>(
        `/ledger/entries?shop_id=${shopId}&from=${from}&to=${to}&limit=20&cursor=${encodeURIComponent(nextCursor)}`,
      );
      setEntries((prev) => [...prev, ...(e.items ?? [])]);
      setNextCursor(e.next_cursor ?? null);
    } catch (e: any) {
      setErrorMsg(e?.message ?? String(e));
    } finally {
      setBusy(false);
    }
  }, [shopId, nextCursor, from, to, apiGet]);

  // --- Actions ---
  async function replaySale(paymentId: number) {
    setBusy(true);
    setErrorMsg(null);
    try {
      await apiPostJson(`/ledger/replay/sale`, { payment_id: paymentId });
      setToast(`replay OK (payment_id=${paymentId})`);
      await loadAll(false);
    } catch (e: any) {
      setErrorMsg(e?.message ?? String(e));
    } finally {
      setBusy(false);
      setTimeout(() => setToast(null), 2500);
    }
  }

  async function createHold() {
    if (!accountId) return;
    setBusy(true);
    setErrorMsg(null);
    try {
      const out = await apiPostJson<{ ok: boolean; hold_id: number }>(
        `/accounts/${accountId}/holds`,
        { amount: holdAmount, currency: "JPY", reason_code: holdReason },
      );
      setLastHoldId(out.hold_id);
      setToast(`hold created (id=${out.hold_id})`);

      const b = await apiGet<Balance>(`/accounts/${accountId}/balance`);
      setBalance(b);
    } catch (e: any) {
      setErrorMsg(e?.message ?? String(e));
    } finally {
      setBusy(false);
      setTimeout(() => setToast(null), 2500);
    }
  }

  async function releaseHold() {
    if (!lastHoldId) return;
    setBusy(true);
    setErrorMsg(null);
    try {
      await apiPostJson(`/holds/${lastHoldId}/release`, {});
      setToast(`hold released (id=${lastHoldId})`);
      setLastHoldId(null);

      if (accountId) {
        const b = await apiGet<Balance>(`/accounts/${accountId}/balance`);
        setBalance(b);
      }
    } catch (e: any) {
      setErrorMsg(e?.message ?? String(e));
    } finally {
      setBusy(false);
      setTimeout(() => setToast(null), 2500);
    }
  }

  async function requestPayout() {
    if (!accountId) return;
    setBusy(true);
    setErrorMsg(null);
    try {
      const out = await apiPostJson<{ ok: boolean; payout_id: number }>(
        `/accounts/${accountId}/payouts`,
        { amount: payoutAmount, currency: "JPY", rail: "manual" },
      );
      setLastPayoutId(out.payout_id);
      setToast(`payout requested (id=${out.payout_id})`);

      const b = await apiGet<Balance>(`/accounts/${accountId}/balance`);
      setBalance(b);
    } catch (e: any) {
      setErrorMsg(e?.message ?? String(e));
    } finally {
      setBusy(false);
      setTimeout(() => setToast(null), 2500);
    }
  }

  async function markPayout(status: "processing" | "paid" | "failed") {
    if (!lastPayoutId) return;
    setBusy(true);
    setErrorMsg(null);
    try {
      await apiPostJson(`/payouts/${lastPayoutId}/status`, { status });
      setToast(`payout ${status} (id=${lastPayoutId})`);
      if (status === "paid" || status === "failed") setLastPayoutId(null);

      if (accountId) {
        const b = await apiGet<Balance>(`/accounts/${accountId}/balance`);
        setBalance(b);
      }
    } catch (e: any) {
      setErrorMsg(e?.message ?? String(e));
    } finally {
      setBusy(false);
      setTimeout(() => setToast(null), 2500);
    }
  }

  // --- Auth gate rendering ---
  if (!authReady || isAuthLoading)
    return <div className="p-6">読み込み中...</div>;
  if (!isAuthenticated) {
    router.replace("/login");
    return null;
  }
  if (!isShopStaff)
    return <div className="p-6">アクセス権限がありません。</div>;
  if (!shopId) return <div className="p-6">shop_id が解決できません。</div>;

  return (
    <div className="p-6 space-y-6">
      {/* Header */}
      <div className="flex items-start justify-between gap-4 flex-wrap">
        <div className="space-y-1">
          <Link
            href={`/shops/${shop_code}/dashboard`}
            className="text-blue-600 underline"
          >
            ← 店舗ダッシュボードへ戻る
          </Link>
          <h1 className="text-3xl font-bold">
            TrustLedger PaymentSystem 管理レビュー（v3）
          </h1>
          <p className="text-sm text-gray-600">
            Shop: {shop_code} / role: {roleInShop ?? "-"} / shop_id: {shopId}
          </p>
        </div>

        <div className="flex items-center gap-2">
          <button
            onClick={() => loadAll(false)}
            className="px-3 py-2 rounded border hover:bg-gray-50 disabled:opacity-50"
            disabled={busy}
          >
            更新
          </button>
        </div>
      </div>

      {/* Toast / Error */}
      {toast && (
        <div className="p-3 rounded border bg-green-50 text-green-800 text-sm">
          {toast}
        </div>
      )}
      {errorMsg && (
        <div className="p-3 rounded border bg-red-50 text-red-700 text-sm whitespace-pre-wrap">
          {errorMsg}
        </div>
      )}

      {/* Filters */}
      <div className="p-4 border rounded bg-white space-y-4">
        <div className="flex flex-wrap items-center gap-3">
          <div className="text-sm font-semibold">期間</div>

          <select
            className="border rounded px-2 py-2 text-sm"
            value={preset}
            onChange={(e) => setPreset(e.target.value as PresetKey)}
          >
            <option value="today">当日</option>
            <option value="w1">1週間</option>
            <option value="w2">2週間</option>
            <option value="m1">1ヶ月</option>
            <option value="m3">3ヶ月</option>
            <option value="m6">半年</option>
            <option value="y1">1年</option>
            <option value="custom">カスタム</option>
          </select>

          <div className="flex items-center gap-2 text-sm">
            <span>from</span>
            <input
              type="date"
              className="border rounded px-2 py-2"
              value={from}
              onChange={(e) => {
                setPreset("custom");
                setFrom(e.target.value);
              }}
            />
            <span>to</span>
            <input
              type="date"
              className="border rounded px-2 py-2"
              value={to}
              onChange={(e) => {
                setPreset("custom");
                setTo(e.target.value);
              }}
            />
          </div>

          <div className="ml-auto flex items-center gap-2">
            <div className="text-sm font-semibold">posting_type</div>
            <select
              className="border rounded px-2 py-2 text-sm"
              value={postingType}
              onChange={(e) =>
                setPostingType(e.target.value as PostingTypeFilter)
              }
            >
              <option value="all">all</option>
              <option value="sale">sale</option>
              <option value="fee">fee</option>
              <option value="refund">refund</option>
            </select>
          </div>
        </div>
      </div>

      {/* KPI cards */}
      <div className="grid gap-4 md:grid-cols-3">
        <div className="p-4 border rounded bg-white">
          <div className="text-xs text-gray-500">Gross Sales</div>
          <div className="text-2xl font-bold">
            {money(summary?.sales_total)}
          </div>
          <div className="text-xs text-gray-500 mt-1">JPY</div>
        </div>

        <div className="p-4 border rounded bg-white">
          <div className="text-xs text-gray-500">Refund Total</div>
          <div className="text-2xl font-bold">
            {money(summary?.refund_total)}
          </div>
          <div className="text-xs text-gray-500 mt-1">JPY</div>
        </div>

        <div className="p-4 border rounded bg-white">
          <div className="text-xs text-gray-500">Fee Total</div>
          <div className="text-2xl font-bold">{money(feeTotalApprox)}</div>
          <div className="text-xs text-gray-500 mt-1">
            ※ entries(最大200件)から推定
          </div>
        </div>

        <div className="p-4 border rounded bg-white">
          <div className="text-xs text-gray-500">Net (sale-refund)</div>
          <div className="text-2xl font-bold">{money(summary?.net_total)}</div>
          <div className="text-xs text-gray-500 mt-1">JPY</div>
        </div>

        <div className="p-4 border rounded bg-white">
          <div className="text-xs text-gray-500">Net (after fee approx)</div>
          <div className="text-2xl font-bold">
            {money(netAfterFeeApprox ?? null)}
          </div>
          <div className="text-xs text-gray-500 mt-1">参考値</div>
        </div>

        <div className="p-4 border rounded bg-white">
          <div className="text-xs text-gray-500">Postings Count</div>
          <div className="text-2xl font-bold">
            {summary ? summary.postings_count.toLocaleString() : "-"}
          </div>
          <div className="text-xs text-gray-500 mt-1">summary </div>
        </div>
      </div>

      {/* Balance + Ops */}
      <div className="grid gap-4 md:grid-cols-2">
        {/* Balance */}
        <div className="p-4 border rounded bg-white space-y-3">
          <div className="flex items-center justify-between">
            <h2 className="font-semibold text-lg">Balance</h2>
            <div className="text-xs text-gray-500">
              account_id: {balance?.account_id ?? "-"} /{" "}
              {balance?.calculated_at ?? "-"}
            </div>
          </div>

          <div className="grid gap-3 md:grid-cols-3">
            <div className="p-3 rounded border bg-gray-50">
              <div className="text-xs text-gray-500">available</div>
              <div className="text-lg font-bold">
                {money(balance?.available_amount)}
              </div>
            </div>
            <div className="p-3 rounded border bg-gray-50">
              <div className="text-xs text-gray-500">held</div>
              <div className="text-lg font-bold">
                {money(balance?.held_amount)}
              </div>
            </div>
            <div className="p-3 rounded border bg-gray-50">
              <div className="text-xs text-gray-500">pending</div>
              <div className="text-lg font-bold">
                {money(balance?.pending_amount)}
              </div>
            </div>
          </div>

          <div className="text-xs text-gray-500">
            ※ recalc は期間(from/to)に依存します（台帳期間の net を balance
            に反映）
          </div>
        </div>

        {/* Ops: Hold / Payout */}
        <div className="p-4 border rounded bg-white space-y-4">
          <h2 className="font-semibold text-lg">Operations</h2>

          {/* Hold */}
          <div className="border rounded p-3 bg-slate-50 space-y-2">
            <div className="font-semibold">Hold（留保）</div>
            <div className="flex flex-wrap gap-2 items-end">
              <label className="text-sm">
                amount
                <input
                  type="number"
                  className="block border rounded px-2 py-2 w-36"
                  value={holdAmount}
                  onChange={(e) => setHoldAmount(Number(e.target.value))}
                  min={1}
                />
              </label>

              <label className="text-sm">
                reason_code
                <select
                  className="block border rounded px-2 py-2 w-52"
                  value={holdReason}
                  onChange={(e) => setHoldReason(e.target.value)}
                >
                  <option value="shipment_pending">shipment_pending</option>
                  <option value="chargeback_risk">chargeback_risk</option>
                  <option value="manual_review">manual_review</option>
                  <option value="other">other</option>
                </select>
              </label>

              <button
                onClick={createHold}
                className="px-3 py-2 rounded border hover:bg-white disabled:opacity-50"
                disabled={busy || !accountId}
              >
                Create Hold
              </button>

              <button
                onClick={releaseHold}
                className="px-3 py-2 rounded border hover:bg-white disabled:opacity-50"
                disabled={busy || !lastHoldId}
                title="最小：直近作成holdのみ解除（一覧APIは将来）"
              >
                Release (last: {lastHoldId ?? "-"})
              </button>
            </div>

            <div className="text-xs text-gray-500">
              ※ Hold一覧APIは将来（運用強化/管理画面）で追加
            </div>
          </div>

          {/* Payout */}
          <div className="border rounded p-3 bg-slate-50 space-y-2">
            <div className="font-semibold">Payout（出金）</div>
            <div className="flex flex-wrap gap-2 items-end">
              <label className="text-sm">
                amount
                <input
                  type="number"
                  className="block border rounded px-2 py-2 w-36"
                  value={payoutAmount}
                  onChange={(e) => setPayoutAmount(Number(e.target.value))}
                  min={1}
                />
              </label>

              <button
                onClick={requestPayout}
                className="px-3 py-2 rounded border hover:bg-white disabled:opacity-50"
                disabled={busy || !accountId}
              >
                Request
              </button>

              <button
                onClick={() => markPayout("processing")}
                className="px-3 py-2 rounded border hover:bg-white disabled:opacity-50"
                disabled={busy || !lastPayoutId}
              >
                processing (id={lastPayoutId ?? "-"})
              </button>

              <button
                onClick={() => markPayout("paid")}
                className="px-3 py-2 rounded border hover:bg-white disabled:opacity-50"
                disabled={busy || !lastPayoutId}
              >
                paid
              </button>

              <button
                onClick={() => markPayout("failed")}
                className="px-3 py-2 rounded border hover:bg-white disabled:opacity-50"
                disabled={busy || !lastPayoutId}
              >
                failed
              </button>
            </div>

            <div className="text-xs text-gray-500">
              ※ Payout一覧APIは将来（運用強化/管理画面）で追加
            </div>
          </div>
        </div>
      </div>

      {/* Reconciliation */}
      <div className="p-4 border rounded bg-white space-y-3">
        <div className="flex items-center justify-between">
          <h2 className="font-semibold text-lg">Reconciliation / Replay</h2>
          {!canOperateReconciliation && (
            <span className="text-xs px-2 py-1 rounded bg-gray-200 text-gray-700">
              owner/manager のみ推奨（現在: {roleInShop ?? "-"}）
            </span>
          )}
        </div>

        {canOperateReconciliation ? (
          <>
            <div className="text-sm text-gray-600">
              欠損検知（paymentsはあるがpostingが無い等）→
              replayで復旧（冪等）。
            </div>

            <div className="text-sm">
              missing_sale_count:{" "}
              <span className="font-bold">
                {recon ? recon.missing_sale_count : "-"}
              </span>
            </div>

            {recon && recon.missing_sale_count > 0 && (
              <div className="border rounded p-3 bg-red-50 space-y-2">
                <div className="text-sm font-semibold text-red-700">
                  Missing sales
                </div>
                <div className="overflow-auto">
                  <table className={styles.table}>
                    <thead className={styles.thead}>
                      <tr>
                        <th className={styles.th}>payment_id</th>
                        <th className={styles.th}>order_id</th>
                        <th className={styles.th}>provider_payment_id</th>
                        <th className={styles.th}>updated_at</th>
                        <th className={styles.th}></th>
                      </tr>
                    </thead>
                    <tbody>
                      {recon.missing_sales.map((m) => (
                        <tr key={m.payment_id} className={styles.tr}>
                          <td className={styles.td}>{m.payment_id}</td>
                          <td className={styles.td}>{m.order_id}</td>
                          <td className={styles.td}>{m.provider_payment_id}</td>
                          <td className={styles.td}>{m.updated_at}</td>
                          <td className={styles.td}>
                            <button
                              className="px-2 py-1 rounded border hover:bg-white disabled:opacity-50"
                              disabled={busy}
                              onClick={() => replaySale(m.payment_id)}
                            >
                              replay
                            </button>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            )}

            {recon && recon.missing_sale_count === 0 && (
              <div className="text-sm text-green-700">欠損なし（OK）</div>
            )}
          </>
        ) : (
          <div className="text-sm text-gray-600">
            この操作は owner/manager のみ表示が推奨です。
          </div>
        )}
      </div>

      {/* Entries */}
      <div className="p-4 border rounded bg-white space-y-3">
        <div className="flex items-center justify-between">
          <h2 className="font-semibold text-lg">Ledger Entries（監査ログ）</h2>
          <div className="text-xs text-gray-500">
            showing: {filteredEntries.length} / loaded: {entries.length}
          </div>
        </div>

        <div className="overflow-auto">
          <table className={styles.table}>
            <thead className={styles.thead}>
              <tr>
                <th className={styles.th}>occurred_at</th>
                <th className={styles.th}>type</th>
                <th className={styles.th}>order_id</th>
                <th className={styles.th}>payment_id</th>
                <th className={styles.th}>source_event_id</th>
                <th className={styles.th} style={{ textAlign: "right" }}>
                  amount
                </th>
                <th className={styles.th}></th>
              </tr>
            </thead>

            <tbody>
              {filteredEntries.map((it) => {
                const expanded = !!expandedPostingIds[it.posting_id];
                const amount =
                  typeof it.amount === "number"
                    ? it.amount
                    : (it.entries ?? [])
                        .filter((l) => l.side === "debit")
                        .reduce((acc, l) => acc + (l.amount ?? 0), 0);

                return (
                  <Fragment key={it.posting_id}>
                    <tr className={styles.tr}>
                      <td className={styles.td}>{it.occurred_at}</td>
                      <td className={styles.td}>
                        <span className="px-2 py-1 rounded bg-gray-100 border text-xs">
                          {it.posting_type}
                        </span>
                      </td>
                      <td className={styles.td}>{it.order_id ?? "-"}</td>
                      <td className={styles.td}>{it.payment_id ?? "-"}</td>
                      <td
                        className={styles.td}
                        style={{
                          fontFamily:
                            "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace",
                        }}
                      >
                        {it.source_event_id}
                      </td>
                      <td className={styles.td} style={{ textAlign: "right" }}>
                        {amount.toLocaleString()}
                      </td>
                      <td className={styles.td} style={{ textAlign: "right" }}>
                        <button
                          className="px-2 py-1 rounded border hover:bg-white"
                          onClick={() => toggleExpand(it.posting_id)}
                        >
                          {expanded ? "close" : "details"}
                        </button>
                      </td>
                    </tr>

                    {expanded && (
                      <tr className={styles.detailRow}>
                        <td className={styles.detailCell} colSpan={7}>
                          <div className="text-sm space-y-2">
                            <div className="text-xs text-gray-500">
                              posting_id: {it.posting_id} / currency:{" "}
                              {it.currency}
                            </div>

                            <div className="overflow-auto">
                              <table className={styles.innerTable}>
                                <thead>
                                  <tr>
                                    <th className={styles.innerTh}>
                                      account_code
                                    </th>
                                    <th className={styles.innerTh}>side</th>
                                    <th
                                      className={styles.innerTh}
                                      style={{ textAlign: "right" }}
                                    >
                                      amount
                                    </th>
                                  </tr>
                                </thead>
                                <tbody>
                                  {it.entries.map((line, idx) => (
                                    <tr
                                      key={`${it.posting_id}-${line.account_code}-${line.side}-${idx}`}
                                    >
                                      <td
                                        className={styles.innerTd}
                                        style={{
                                          fontFamily:
                                            "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace",
                                        }}
                                      >
                                        {line.account_code}
                                      </td>
                                      <td className={styles.innerTd}>
                                        {line.side}
                                      </td>
                                      <td
                                        className={styles.innerTd}
                                        style={{ textAlign: "right" }}
                                      >
                                        {line.amount.toLocaleString()}
                                      </td>
                                    </tr>
                                  ))}
                                </tbody>
                              </table>
                            </div>

                            <div className="text-xs text-gray-500">
                              ※ double-entry の証跡（debit/credit）をここで確認
                            </div>
                          </div>
                        </td>
                      </tr>
                    )}
                  </Fragment>
                );
              })}

              {filteredEntries.length === 0 && (
                <tr>
                  <td className={styles.td} colSpan={7}>
                    <div className="text-sm text-gray-500">
                      該当データがありません。
                    </div>
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>

        <div className="flex items-center justify-between pt-2">
          <div className="text-xs text-gray-500">
            ※ Fee KPIは entries(最大200件)の推定。将来 summary側で fee_total
            を追加すると完全になります。
          </div>

          <button
            className="px-3 py-2 rounded border hover:bg-gray-50 disabled:opacity-50"
            disabled={busy || !nextCursor}
            onClick={loadMore}
            title={!nextCursor ? "これ以上ありません" : "次を読み込む"}
          >
            もっと読む
          </button>
        </div>
      </div>
    </div>
  );
}
