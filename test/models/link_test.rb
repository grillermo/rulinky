# frozen_string_literal: true

require "test_helper"

class LinkTest < ActiveSupport::TestCase
  def teardown
    Link.delete_all
    User.delete_all
  end

  test "belongs to a user" do
    user = User.create!(email: "owner@example.com")
    ts = (Time.now.to_f * 1000).to_i
    link = Link.create!(id: SecureRandom.uuid, url: "https://example.com",
                        read: 0, timestamp: ts, updated_at: ts, user: user)
    assert_equal user, link.reload.user
  end
end
