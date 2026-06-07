# frozen_string_literal: true

class AddUserToLinks < ActiveRecord::Migration[8.1]
  def change
    add_reference :links, :user, foreign_key: true
  end
end
