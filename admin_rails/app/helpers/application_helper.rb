module ApplicationHelper
  def fmt_jst(ts)
    return "-" if ts.blank?

    t =
      case ts
      when Time, DateTime, ActiveSupport::TimeWithZone
        ts
      else
        s = ts.to_s
        # ISO8601優先。ダメなら Time.zone.parse にフォールバック
        Time.iso8601(s) rescue Time.zone.parse(s) rescue nil
      end

    return ts.to_s if t.nil?

    t.in_time_zone("Asia/Tokyo").strftime("%Y-%m-%d %H:%M:%S")
  end
end