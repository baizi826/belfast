-- Ship collection grouping.
--
-- SC_17001's `ship_info_list[].id` must be a group_type (a key of the client's
-- ship_data_group.get_id_list_by_group_type), NOT `owned_ships.ship_id / 10`.
-- Verified against the official reply: 793/793 of its ids are group_types, while
-- `ship_id / 10` only matches for "prototype" ids -- refit ships break it, e.g.
--
--     ship_data_template.get_id_list_by_group_type
--     [10126] = { 101261, 101262, 101263, 101264, ..., 101994, 900431 }
--                                         ^ /10 = 10126 OK   ^ /10 = 10199 WRONG
--
-- The client resolves every entry through pg.ship_data_group[id]; an unknown id makes
-- ShipGroup.__index raise and the login-time painting check (LoginMediator
-- :checkPaintingRes -> PaintingGroupConst:GetPaintingNameListInLogin -> CollectionProxy
-- :getGroups) abort, so the client never reaches the main screen.
--
-- skins.ship_group lives in the same number space (verified: range 10000..1170002, and
-- 0 rows carry any of the bad ids while 84 carry the correct ones), so every place that
-- related owned_ships to skins or counted collection groups was wrong the same way.
--
-- Values are populated from <BELFAST_DATA_DIR>/ship_group_types.json at startup
-- (misc.BackfillShipGroupTypes), so a reseed self-heals instead of leaving the column
-- unusable. Deliberately NOT backfilled as `template_id / 10` here -- that expression
-- is exactly the bug this column exists to replace.
ALTER TABLE ships ADD COLUMN IF NOT EXISTS group_type bigint;

CREATE INDEX IF NOT EXISTS ships_group_type_idx ON ships (group_type);
