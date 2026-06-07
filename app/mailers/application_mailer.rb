# frozen_string_literal: true

class ApplicationMailer < ActionMailer::Base
  default from: ENV["DEFAULT_EMAIL"].presence || "envio@readitsoon.app"
  layout "mailer"
end
