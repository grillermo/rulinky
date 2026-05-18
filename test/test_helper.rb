# frozen_string_literal: true

ENV["RAILS_ENV"] ||= "test"
require_relative "../config/environment"
require "rails/test_help"

module ActiveSupport
  class TestCase
    fixtures :all
  end
end

class ActionDispatch::IntegrationTest
  MODERN_UA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

  %w[get post patch put head delete].each do |method|
    define_method(method) do |path, **args|
      args[:headers] = { "User-Agent" => MODERN_UA }.merge(args[:headers] || {})
      super(path, **args)
    end
  end
end
