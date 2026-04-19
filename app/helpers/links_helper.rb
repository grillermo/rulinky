# frozen_string_literal: true

module LinksHelper
  def ms_to_local_time_string(ms)
    milliseconds = ms.to_i
    return "" if milliseconds <= 0

    Time.at(milliseconds / 1000.0).getlocal.strftime("%-m/%-d/%Y, %-I:%M:%S %p")
  end

  def display_url_without_scheme(url)
    return "" if url.blank?

    uri = URI.parse(url)
    return url unless uri.host

    host = uri.host.sub(/\Awww\./i, "")
    formatted_url = +"#{host}"
    formatted_url << ":#{uri.port}" if uri.port && uri.port != uri.default_port
    formatted_url << uri.path if uri.path.present?
    formatted_url << "##{uri.fragment}" if uri.fragment.present?
    formatted_url
  rescue URI::InvalidURIError
    url.sub(/\Ahttps?:\/\/(www\.)?/i, "").sub(/\?.*\z/, "")
  end

  def truncated_display_url(url, max_length: 30)
    display_url_without_scheme(url).truncate(max_length)
  end
end
