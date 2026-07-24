UPDATE OR IGNORE user_items         SET item='item2'      WHERE item='item1';
DELETE FROM      user_items         WHERE item='item1';

UPDATE OR IGNORE user_item_multiply SET item='item2'      WHERE item='item1';
DELETE FROM      user_item_multiply WHERE item='item1';

UPDATE OR IGNORE user_last_used     SET item='item2'      WHERE item='item1';
DELETE FROM      user_last_used     WHERE item='item1';

UPDATE           user_equipped      SET item_name='item2' WHERE item_name='item1';

UPDATE OR IGNORE items              SET name='item2'      WHERE name='item1';
DELETE FROM      items              WHERE name='item1';
