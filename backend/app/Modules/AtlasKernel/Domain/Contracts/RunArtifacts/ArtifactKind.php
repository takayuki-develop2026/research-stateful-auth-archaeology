<?php
declare(strict_types=1);

namespace App\Modules\AtlasKernel\Domain\Contracts\RunArtifacts;

final class ArtifactKind
{
    // ----------------------------
    // v1: UIでまず確実に出したい候補
    // ----------------------------

    // RunArtifacts v1.0 で「契約的に」よく使うもの（必要に応じて追加）
    public const EVIDENCE_ASSET         = 'evidence.asset';
    public const DIFF_JSON              = 'diff.json';
    public const REVIEW_SNAPSHOT_BEFORE = 'review.snapshot.before';
    public const REVIEW_SNAPSHOT_AFTER  = 'review.snapshot.after';
    public const DECISION_PAYLOAD       = 'decision.payload';
    public const POLICY_EVALUATION      = 'policy.evaluation';

    // v3.2 現状運用で“必ず”出ているもの（UI候補に無いとフィルタが死ぬ）
    public const ANALYSIS_RESULT        = 'analysis_result';
    public const ROUTE_DECISION         = 'route_decision';
    public const ATTEMPT_STATE          = 'attempt_state';
    public const REVIEW_REQUIRED_REASON = 'review_required_reason';

    /**
     * RunArtifacts v1.0 artifact_kind format (DBと完全一致)
     *
     * - lowercase
     * - snake_case OK（例: analysis_result）
     * - dot.notation OK（例: review.snapshot.after）
     * - 先頭は英小文字
     * - セグメント内は [a-z0-9_]
     */
    private const REGEX = '/^[a-z][a-z0-9_]*(\.[a-z0-9_]+)*$/';

    public static function assertValid(string $kind): void
    {
        $kind = trim($kind);
        if ($kind === '') {
            throw new \InvalidArgumentException('artifact_kind is required');
        }
        if (!preg_match(self::REGEX, $kind)) {
            throw new \InvalidArgumentException("artifact_kind invalid format: {$kind}");
        }
    }

    /**
     * UI側が候補を出すためのリスト（ただし未知kindも許可する思想）
     */
    public static function knownKinds(): array
    {
        return [
            // v3.2 “現実に出る順” で先に（UIで使いやすい）
            self::ANALYSIS_RESULT,
            self::ROUTE_DECISION,
            self::ATTEMPT_STATE,
            self::REVIEW_REQUIRED_REASON,

            // v1 契約でよく使う代表
            self::EVIDENCE_ASSET,
            self::DIFF_JSON,
            self::REVIEW_SNAPSHOT_BEFORE,
            self::REVIEW_SNAPSHOT_AFTER,
            self::DECISION_PAYLOAD,
            self::POLICY_EVALUATION,
        ];
    }
}