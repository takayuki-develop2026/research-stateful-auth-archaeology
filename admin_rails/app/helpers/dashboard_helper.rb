module DashboardHelper
  def dash_action(label, href, enabled:, style: :ghost, note: nil)
    css = ["btn", "btn--#{style}"]
    if enabled
      link_to label, href, class: css.join(" ")
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
    return {} unless enabled
    { onclick: "window.location='#{href}'", role: "link", tabindex: "0", class: "is-clickable" }
  end
end