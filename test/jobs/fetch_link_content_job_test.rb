# frozen_string_literal: true

require "test_helper"

class FetchLinkContentJobTest < ActiveSupport::TestCase
  def setup
    @link = Link.create!(
      id: SecureRandom.uuid,
      url: "https://example.com/post",
      timestamp: (Time.now.to_f * 1000).to_i,
      updated_at: (Time.now.to_f * 1000).to_i,
    )
    @job = LinkContentJob.create!(id: SecureRandom.uuid, link: @link, status: "queued")
  end

  def teardown
    LinkContentJob.delete_all
    Link.delete_all
  end

  # Temporarily replaces a class method, restoring the original afterwards.
  def with_class_stub(klass, name, impl)
    original = klass.method(name)
    klass.define_singleton_method(name, impl)
    yield
  ensure
    klass.define_singleton_method(name, original)
  end

  test "crawls, previews, and stores content and raw_title" do
    with_class_stub(ChromeCrawler, :call, ->(_url) { "<html>cleaned</html>" }) do
      with_class_stub(ReaditsoonPreview, :call, ->(_html, _url) { { title: "Clean Title", content: "<p>Body</p>" } }) do
        FetchLinkContentJob.new.perform(@job.id)
      end
    end

    @link.reload
    @job.reload
    assert_equal "<p>Body</p>", @link.content
    assert_equal "Clean Title", @link.raw_title
    assert_equal "finished", @job.status
  end

  test "marks job failed when crawl raises" do
    with_class_stub(ChromeCrawler, :call, ->(_url) { raise "chrome exploded" }) do
      FetchLinkContentJob.new.perform(@job.id)
    end

    @job.reload
    assert_equal "failed", @job.status
    assert_match(/chrome exploded/, @job.error_message)
  end
end
