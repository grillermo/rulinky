# frozen_string_literal: true

class LinksController < ApplicationController
  def index
    @filter = params[:filter] == "read" ? "read" : "unread"

    scope = Link.includes(:content_jobs).order(updated_at: :desc).select(:id, :url, :note, :read, :timestamp, :updated_at, :content)
    @links = scope.to_a

    @read_links_count = @links.count { |link| link.read.to_i == 1 }
    @unread_links_count = @links.count { |link| link.read.to_i == 0 }

    render inertia: "Links/Index", props: {
      links: @links.map { |l|
        {
          id: l.id,
          url: l.url,
          title: helpers.link_display_title(l),
          fullTitle: helpers.link_display_title_full(l),
          note: l.note,
          read: l.read.to_i == 1,
          updatedAt: helpers.ms_to_local_time_string(l.updated_at),
          activeJobId: l.active_content_job&.id
        }
      },
      readCount: @read_links_count,
      unreadCount: @unread_links_count
    }
  end

  def destroy
    link = Link.find_by(id: params[:id])
    link&.destroy
    redirect_to root_path
  end

  def read
    update_read_status(1)
  end

  def unread
    update_read_status(0)
  end

  private

  def update_read_status(read_value)
    link = Link.find_by(id: params[:id])
    link&.update!(read: read_value, updated_at: (Time.now.to_f * 1000).to_i)
    redirect_back(fallback_location: root_path(filter: read_value == 1 ? "unread" : "read"))
  end
end
