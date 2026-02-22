# frozen_string_literal: true

module LinksHelper
  def ms_to_local_time_string(ms)
    milliseconds = ms.to_i
    return "" if milliseconds <= 0

    Time.at(milliseconds / 1000.0).getlocal.strftime("%-m/%-d/%Y, %-I:%M:%S %p")
  end
end

