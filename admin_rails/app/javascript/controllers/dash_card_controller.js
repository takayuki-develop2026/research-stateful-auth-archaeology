import { Controller } from "@hotwired/stimulus";

/**
 * dash-card controller
 *
 * 目的:
 * - カード全体クリックで遷移できるようにする
 * - ただしカード内の a/button/form などをクリックした時は「カード遷移」を発火させない
 *
 * 使い方:
 * - data-controller="dash-card"
 * - data-action="click->dash-card#go"
 * - data-dash-card-href-value="/path"
 */
export default class extends Controller {
  static values = {
    href: String,
    enabled: Boolean,
  };

  go(e) {
    // 無効化カードは何もしない（disabled card）
    if (this.hasEnabledValue && this.enabledValue === false) {
      return;
    }

    // 子のリンク/ボタン/フォーム操作ならカード遷移しない
    // ここが今回の修正Aの本体
    if (
      e.target.closest(
        "a, button, input, select, textarea, form, [data-card-nav-ignore='true']",
      )
    ) {
      return;
    }

    // 通常のカードクリックだけ遷移
    const href = this.hasHrefValue ? this.hrefValue : null;
    if (!href || href.length === 0) {
      return;
    }

    window.location.href = href;
  }
}
