"use client";

import { useEffect, useMemo, useState } from "react";
import { useAuth } from "@/ui/auth/AuthProvider";

export default function VerifyEmailPage() {
  const { user, apiClient, isLoading } = useAuth();
  const [statusMessage, setStatusMessage] = useState<string | null>(null);
  const [isResending, setIsResending] = useState(false);

  // ✅ 入力メール（登録時に保存したもの）を表示するための受け皿
  const [pendingEmail, setPendingEmail] = useState<string | null>(null);

  useEffect(() => {
    // 登録フォーム側で localStorage.setItem("pending_email", email) しておく前提
    const v = localStorage.getItem("pending_email");
    if (v) setPendingEmail(v);
  }, []);

  // ✅ 表示だけ差し替え（機能はそのまま）
  const displayEmail = useMemo(() => {
    return user?.email ?? pendingEmail ?? "";
  }, [user?.email, pendingEmail]);

  const handleResend = async () => {
    if (!apiClient) return;

    setIsResending(true);
    setStatusMessage(null);

    try {
      await apiClient.post("/email/verification-notification");
      setStatusMessage("認証メールを再送しました。");
    } catch {
      setStatusMessage("再送に失敗しました。");
    } finally {
      setIsResending(false);
    }
  };

  if (isLoading) {
    return <div className="mt-20 text-center">読み込み中...</div>;
  }

  return (
    <div className="min-h-screen flex justify-center items-start pt-20 bg-gray-50">
      <div className="w-full max-w-xl p-8 bg-white rounded-lg shadow-xl">
        <h2 className="text-3xl font-bold text-indigo-600 text-center mb-6">
          メール認証のご案内
        </h2>

        <p className="text-center text-gray-700">
          <strong>{displayEmail}様</strong>
          <br />
          meilhogのダッシュボード宛に登録した認証メールを送信しました。
        </p>

        <p className="mt-3 text-center text-gray-600">
          登録メール宛のリンクをクリックして認証を完了してください。
          <br />
          <br />
          その後Auth OのログインページのSign upを押して
          <br />
          メールとパスワードを入力してAuth Oに登録してください。
          <br />
          <br />
          その後登録メール宛にAuth Oから
          <br />
          ”Verify your email”が届くのでAccountを確認して、
          <br />
          ”Verify Link” or ”Verify Your Account”を押して
          <br />
          認証を完了してログインしてください。
        </p>

        {/* MailHog link */}
        <div className="mt-4 text-center">
          <a
            href="http://localhost:8025/"
            target="_blank"
            rel="noopener noreferrer"
            className="text-indigo-600 font-semibold underline"
          >
            MailHogを開く（http://localhost:8025/）
          </a>
        </div>

        {statusMessage && (
          <div className="mt-6 p-3 bg-blue-50 text-blue-700 rounded text-center">
            {statusMessage}
          </div>
        )}

        <button
          onClick={handleResend}
          disabled={isResending || !user}
          className="mt-6 w-full bg-indigo-600 text-white py-3 rounded font-bold disabled:opacity-60"
        >
          認証メールを再送
        </button>
      </div>
    </div>
  );
}
