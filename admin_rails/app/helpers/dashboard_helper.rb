module DashboardHelper
  def dash_action(label, href, enabled:, style: :ghost, note: nil)
    css = ["btn", "btn--#{style}"]

    if enabled
      # ✅ カードの click handler に拾われないように「無視フラグ」を付与（保険）
      link_to label, href, class: css.join(" "), data: { card_nav_ignore: "true" }
    else
      content_tag(
        :span,
        label,
        class: (css + ["is-disabled"]).join(" "),
        role: "button",
        tabindex: "-1",
        "aria-disabled": "true",
        title: note || "準備中"
      )
    end
  end

  def dash_card_link(enabled:, href:)
    # ✅ disabled でも data は付けておく（JS側で enabled=false なら return する）
    {
      "data-controller": "dash-card",
      "data-action": "click->dash-card#go",
      "data-dash-card-href-value": href.to_s,
      "data-dash-card-enabled-value": (!!enabled).to_s,
      role: "link",
      tabindex: enabled ? "0" : "-1",
      "aria-disabled": (!enabled).to_s,
      class: (enabled ? "is-clickable" : nil),
    }.compact
  end
end