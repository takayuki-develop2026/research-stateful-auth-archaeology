import { Controller } from "@hotwired/stimulus";

export default class extends Controller {
  static targets = ["kind", "q"];

  connect() {
    // no-op: server-rendered
  }

  // optional: フォーム submit 前に軽く正規化
  submit(event) {
    const kind = (this.kindTarget?.value || "").trim().toLowerCase();
    const q = (this.qTarget?.value || "").trim();

    // kind: dot-separated lowercase only (allow empty)
    if (kind && !/^[a-z0-9]+(\.[a-z0-9]+)*$/.test(kind)) {
      // 壊れない運用：不正kindは空扱いにして落とす
      this.kindTarget.value = "";
    } else {
      this.kindTarget.value = kind;
    }

    // q: 長すぎる入力は切る（URL事故防止）
    if (q.length > 200) this.qTarget.value = q.slice(0, 200);
  }
}
