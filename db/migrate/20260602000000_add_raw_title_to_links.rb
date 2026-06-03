# frozen_string_literal: true

class AddRawTitleToLinks < ActiveRecord::Migration[8.1]
  def change
    add_column :links, :raw_title, :text
  end
end
