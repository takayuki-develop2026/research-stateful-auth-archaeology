<?php
declare(strict_types=1);

namespace App\Modules\AtlasKernel\Application\Contracts\RunArtifacts\V1;

final readonly class RunArtifactContentV1
{
    public const SCHEMA_VERSION = '1.0';

    /** @var array<string,mixed> */
    public array $extra;

    /**
     * @param array<string,mixed> $artifact_ref
     * @param array<string,mixed> $produced_by
     * @param array<int,mixed>    $evidence_refs
     * @param array<string,mixed> $trace
     * @param array<string,mixed> $extra
     */
    public function __construct(
        public string $schema_version,
        public array $artifact_ref,
        public array $produced_by,
        public string $policy_version,
        public string $pipeline_version,
        public array $evidence_refs,
        public array $trace,
        array $extra = [],
    ) {
        $this->extra = $extra;
    }

    /**
     * @param array<string,mixed> $json
     */
    public static function fromArray(array $json): self
    {
        $known = [
            'schema_version',
            'artifact_ref',
            'produced_by',
            'policy_version',
            'pipeline_version',
            'evidence_refs',
            'trace',
        ];
        $extra = array_diff_key($json, array_flip($known));

        return new self(
            schema_version: (string)($json['schema_version'] ?? ''),
            artifact_ref: is_array($json['artifact_ref'] ?? null) ? $json['artifact_ref'] : [],
            produced_by: is_array($json['produced_by'] ?? null) ? $json['produced_by'] : [],
            policy_version: (string)($json['policy_version'] ?? ''),
            pipeline_version: (string)($json['pipeline_version'] ?? ''),
            evidence_refs: is_array($json['evidence_refs'] ?? null) ? $json['evidence_refs'] : [],
            trace: is_array($json['trace'] ?? null) ? $json['trace'] : [],
            extra: $extra,
        );
    }

    /**
     * One-shot invariant validation.
     * ここだけ通れば「契約として正しい」。
     */
    public function validateOrThrow(string $artifactKind, string $runId, string $traceId): void
    {
        // 1) schema_version 固定
        if ($this->schema_version !== self::SCHEMA_VERSION) {
            throw new \InvalidArgumentException("schema_version must be '".self::SCHEMA_VERSION."'");
        }

        // 2) 必須キー：artifact_ref
        foreach (['id','kind','run_id','trace_id'] as $k) {
            if (!isset($this->artifact_ref[$k]) || !is_string($this->artifact_ref[$k]) || trim($this->artifact_ref[$k]) === '') {
                throw new \InvalidArgumentException("artifact_ref.$k is required");
            }
        }

        // 3) 必須キー：produced_by
        foreach (['type','name'] as $k) {
            if (!isset($this->produced_by[$k]) || !is_string($this->produced_by[$k]) || trim($this->produced_by[$k]) === '') {
                throw new \InvalidArgumentException("produced_by.$k is required");
            }
        }
        $allowedProducedBy = ['system','user','job','tool'];
        if (!in_array($this->produced_by['type'], $allowedProducedBy, true)) {
            throw new \InvalidArgumentException("produced_by.type invalid");
        }

        // 4) policy_version / pipeline_version
        if (trim($this->policy_version) === '' || trim($this->pipeline_version) === '') {
            throw new \InvalidArgumentException("policy_version and pipeline_version are required");
        }

        // 5) trace
        if (!isset($this->trace['trace_id']) || !is_string($this->trace['trace_id']) || trim($this->trace['trace_id']) === '') {
            throw new \InvalidArgumentException("trace.trace_id is required");
        }

        // 6) evidence_refs 配列・要素検証
        if (!is_array($this->evidence_refs)) {
            throw new \InvalidArgumentException("evidence_refs must be array");
        }
        $allowedEvidenceType = ['html','pdf','image','text','api_response','log','unknown'];
        $refRe = '/^(sha256:[a-f0-9]{64}|s3:\/\/.+|https:\/\/.+|db:.+)$/i';
        $sha256Re = '/^[a-f0-9]{64}$/i';

        foreach ($this->evidence_refs as $i => $ref) {
            if (!is_array($ref)) {
                throw new \InvalidArgumentException("evidence_refs[$i] must be object");
            }
            if (!isset($ref['ref']) || !is_string($ref['ref']) || trim($ref['ref']) === '') {
                throw new \InvalidArgumentException("evidence_refs[$i].ref is required");
            }
            if (!preg_match($refRe, $ref['ref'])) {
                throw new \InvalidArgumentException("evidence_refs[$i].ref invalid (sha256:/s3://https://db:)");
            }

            if (!isset($ref['type']) || !is_string($ref['type']) || trim($ref['type']) === '') {
                throw new \InvalidArgumentException("evidence_refs[$i].type is required");
            }
            if (!in_array($ref['type'], $allowedEvidenceType, true)) {
                throw new \InvalidArgumentException("evidence_refs[$i].type invalid");
            }

            if (isset($ref['sha256'])) {
                if (!is_string($ref['sha256']) || !preg_match($sha256Re, $ref['sha256'])) {
                    throw new \InvalidArgumentException("evidence_refs[$i].sha256 invalid");
                }
            }
            if (isset($ref['size_bytes'])) {
                if (!is_int($ref['size_bytes']) || $ref['size_bytes'] < 0) {
                    throw new \InvalidArgumentException("evidence_refs[$i].size_bytes invalid");
                }
            }
        }

        // 7) Cross-field invariants（最重要）
        if ($this->artifact_ref['kind'] !== $artifactKind) {
            throw new \InvalidArgumentException("artifact_ref.kind mismatch");
        }
        if ($this->artifact_ref['run_id'] !== $runId) {
            throw new \InvalidArgumentException("artifact_ref.run_id mismatch");
        }
        if ($this->trace['trace_id'] !== $traceId) {
            throw new \InvalidArgumentException("trace.trace_id mismatch");
        }
        if ($this->artifact_ref['trace_id'] !== $traceId) {
            throw new \InvalidArgumentException("artifact_ref.trace_id mismatch");
        }
    }

    /**
     * @return array<string,mixed>
     */
    public function toArray(): array
    {
        return [
            'schema_version' => $this->schema_version,
            'artifact_ref' => $this->artifact_ref,
            'produced_by' => $this->produced_by,
            'policy_version' => $this->policy_version,
            'pipeline_version' => $this->pipeline_version,
            'evidence_refs' => $this->evidence_refs,
            'trace' => $this->trace,
            ...$this->extra,
        ];
    }
}
