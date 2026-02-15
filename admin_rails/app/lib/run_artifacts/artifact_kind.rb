# app/lib/run_artifacts/artifact_kind.rb
# frozen_string_literal: true

module RunArtifacts
  module ArtifactKind
    # UI候補としての「既知kind」
    # ※ ここに無いkindが来ても “拒否しない” のが運用強度
    CATALOG = {
      "evidence.asset" => "Evidence (asset)",
      "diff.json" => "Diff (json)",
      "review.snapshot.before" => "Review snapshot (before)",
      "review.snapshot.after" => "Review snapshot (after)",
      "decision.payload" => "Decision payload",
    }.freeze

    KIND_RE = /\A[a-z0-9]+(\.[a-z0-9]+)*\z/.freeze
    MAX_LEN = 120

    module_function

    def known_kinds
      CATALOG.keys
    end

    def label(kind)
      CATALOG[kind] || kind.to_s
    end

    # URL/フォーム入力から安全なkind文字列へ正規化
    # - nil/空は nil
    # - lower + trim
    # - 不正文字は nil（“invalid”扱いにできる）
    def normalize(raw)
      s = raw.to_s.strip.downcase
      return nil if s.empty?
      return nil if s.length > MAX_LEN
      return nil unless KIND_RE.match?(s)
      s
    end

    # UIは「known」だけ許可、にしたい場合のバリデータ（任意）
    def known?(kind)
      CATALOG.key?(kind)
    end
  end
end