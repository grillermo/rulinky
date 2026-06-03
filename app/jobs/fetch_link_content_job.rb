# frozen_string_literal: true

class FetchLinkContentJob
  include SuckerPunch::Job

  def perform(job_id)
    Rails.logger.info("[FetchLinkContentJob] start job_id=#{job_id}")

    job = LinkContentJob.find_by(id: job_id)
    unless job
      Rails.logger.warn("[FetchLinkContentJob] job not found job_id=#{job_id}")
      return
    end

    job.update!(status: "running", error_message: nil)
    Rails.logger.info("[FetchLinkContentJob] status=running job_id=#{job_id}")

    link = job.link
    Rails.logger.info("[FetchLinkContentJob] crawling url=#{link.url}")
    html = ChromeCrawler.call(link.url)
    Rails.logger.info("[FetchLinkContentJob] crawl done html_bytes=#{html&.bytesize}")

    Rails.logger.info("[FetchLinkContentJob] calling ReaditsoonPreview url=#{link.url}")
    result = ReaditsoonPreview.call(html, link.url)
    Rails.logger.info("[FetchLinkContentJob] preview done title=#{result[:title].inspect} content_bytes=#{result[:content]&.bytesize}")

    link.update!(content: result[:content], raw_title: result[:title])
    Rails.logger.info("[FetchLinkContentJob] link updated link_id=#{link.id}")

    job.update!(status: "finished")
    Rails.logger.info("[FetchLinkContentJob] finished job_id=#{job_id}")
  rescue StandardError => e
    Rails.logger.error("[FetchLinkContentJob] failed job_id=#{job_id} error=#{e.class}: #{e.message}")
    Rails.logger.error(e.backtrace.first(5).join("\n"))
    job&.update(status: "failed", error_message: e.message)
  end
end
