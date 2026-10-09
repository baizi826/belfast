-- Bag capacity comes from the account, not from the server.
--
-- player_info.go used to hardcode ShipBagMax/EquipBagMax/CommanderBagMax = 250. Measured
-- against this account's captured official SC_11003 (save/bili/state/req11001_rep11003_01.bin,
-- decode with `go run ./cmd/dumprep -file <bin> -cmd 11003`):
--
--     shipBagMax 1190    equipBagMax 300    commanderBagMax 40
--
-- The wrong equipment capacity is not cosmetic. The account owns 257 equipment, so a cap of
-- 250 made the client treat the equipment bag as full - and it then refuses to unequip
-- anything, because unequipping puts the item back in the bag. The user-visible symptom was
-- "can't remove equipment", with nothing wrong in the equipment code at all.
--
-- The DEFAULT keeps the old 250 so an account with no imported capacity behaves exactly as
-- before instead of acquiring a cap of 0.
ALTER TABLE commanders ADD COLUMN IF NOT EXISTS ship_bag_max      bigint NOT NULL DEFAULT 250;
ALTER TABLE commanders ADD COLUMN IF NOT EXISTS equip_bag_max     bigint NOT NULL DEFAULT 250;
ALTER TABLE commanders ADD COLUMN IF NOT EXISTS commander_bag_max bigint NOT NULL DEFAULT 250;
