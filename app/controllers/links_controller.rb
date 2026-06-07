# frozen_string_literal: true

class LinksController < ApplicationController
  def index
    @filter = params[:filter] == "read" ? "read" : "unread"

    scope = current_user.links.includes(:content_jobs).order(updated_at: :desc).select(
      :id,
      :url,
      :note,
      :read,
      :timestamp,
      :updated_at,
      :content,
      :raw_title
    )
    @links = scope.to_a

    @read_links_count = @links.count { |link| link.read.to_i == 1 }
    @unread_links_count = @links.count { |link| link.read.to_i == 0 }

    render inertia: "Links/Index", props: {
      links: @links.map { |link|
        {
          id: link.id,
          url: link.url,
          title: helpers.link_display_title(link),
          fullTitle: helpers.link_display_title_full(link),
          note: link.note,
          read: link.read.to_i == 1,
          updatedAt: helpers.ms_to_local_time_string(link.updated_at)
        }
      },
      readCount: @read_links_count,
      unreadCount: @unread_links_count
    }
  end

  def destroy
    link = current_user.links.find_by(id: params[:id])
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
    link = current_user.links.find_by(id: params[:id])
    link&.update!(read: read_value, updated_at: (Time.now.to_f * 1000).to_i)
    redirect_back(fallback_location: root_path(filter: read_value == 1 ? "unread" : "read"))
  end
end
