# frozen_string_literal: true

class AddUniqueIndexLinksOnId < ActiveRecord::Migration[8.1]
  def change
    add_index :links, :id, unique: true, name: :index_links_on_id
  end
end

