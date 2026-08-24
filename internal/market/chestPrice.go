package market

import "strconv"

func ChestPrice(treasureType string, dungeonTier int, rewards []string) int {
	price := BaseChestPrice[treasureType]

	rewardPrices := ChestPriceItems[treasureType][strconv.Itoa(dungeonTier)]
	for _, reward := range rewards {
		price += rewardPrices[reward]
	}
	return price
}

var BaseChestPrice = map[string]int{
	"bedrock":  2_000_000,
	"obsidian": 1_000_000,
	"emerald":  500_000,
	"diamond":  250_000,
	"gold":     100_000,
}

var ChestPriceItems = map[string]map[string]map[string]int{
	"bedrock": {
		"7": {
			"necron_handle":        98_000_000,
			"implosion_scroll":     48_000_000,
			"wither_shield_scroll": 48_000_000,
			"shadow_warp_scroll":   48_000_000,
			"auto_recombobulator":  8_000_000,
			"wither_chestplate":    8_000_000,
			"recombobulator_3000":  4_000_000,
			"wither_leggings":      4_000_000,
			"wither_cloak":         2_500_000,
			"wither_blood":         1_000_000,
			"wither_helmet":        2_000_000,
			"wither_boots":         500_000,
			"fifth_master_star":    7_000_000,
		},
		"6": {
			"precursor_eye":               28_000_000,
			"giants_sword":                23_000_000,
			"necromancer_lord_chestplate": 8_000_000,
			"summoning_ring":              10_000_000,
			"fel_skull":                   4_000_000,
			"necromancer_sword":           8_000_000,
			"necromancer_lord_leggings":   2_000_000,
			"recombobulator_3000":         4_000_000,
			"soulweaver_gloves":           3_000_000,
			"fourth_master_star":          6_000_000,
		},
		"5": {
			"shadow_fury":                13_000_000,
			"last_breath":                5_000_000,
			"shadow_assassin_chestplate": 4_000_000,
			"livid_dagger":               5_000_000,
			"shadow_assassin_cloak":      500_000,
			"shadow_assassin_leggings":   2_000_000,
			"aote_stone":                 3_000_000,
			"recombobulator_3000":        4_000_000,
			"third_master_star":          5_000_000,
		},
	},
	"obsidian": {
		"7": {
			"wither_chestplate":   9_000_000,
			"one_for_all_1":       1_000_000,
			"wither_leggings":     5_000_000,
			"recombobulator_3000": 5_000_000,
			"wither_cloak":        3_500_000,
			"wither_blood":        1_500_000,
			"wither_helmet":       3_000_000,
			"wither_boots":        1_500_000,
			"fifth_master_star":   8_000_000,
		},
		"6": {
			"summoning_ring":            11_000_000,
			"necromancer_sword":         9_000_000,
			"necromancer_lord_leggings": 3_000_000,
			"recombobulator_3000":       5_000_000,
			"soulweaver_gloves":         4_000_000,
			"necromancer_lord_helmet":   1_000_000,
			"sadan_brooch":              500_000,
			"necromancer_lord_boots":    500_000,
			"fourth_master_star":        7_000_000,
		},
		"5": {
			"livid_dagger":             6_000_000,
			"shadow_assassin_cloak":    1_500_000,
			"shadow_assassin_leggings": 3_000_000,
			"aote_stone":               4_000_000,
			"recombobulator_3000":      5_000_000,
			"shadow_assassin_helmet":   1_000_000,
			"shadow_assassin_boots":    500_000,
		},
		"4": {
			"thorns_boots":        3_000_000,
			"item_spirit_bow":     3_000_000,
			"spirit_sword":        2_000_000,
			"recombobulator_3000": 5_000_000,
			"spirit_wing":         1_000_000,
			"spirit_bone":         500_000,
			"fuming_potato_book":  500_000,
			// "spirit_pet":       4_000_000,
			"spirit_decoy":       500_000,
			"second_master_star": 5_000_000,
		},
		"3": {
			"adaptive_chestplate": 1_000_000,
			"recombobulator_3000": 5_000_000,
			"fuming_potato_book":  500_000,
			"first_master_star":   4_000_000,
		},
		"2": {
			"stone_blade":         1_000_000,
			"recombobulator_3000": 5_000_000,
			"fuming_potato_book":  750_000,
		},
		"1": {
			"recombobulator_3000": 5_000_000,
			"bonzo_staff":         2_000_000,
			"fuming_potato_book":  1_000_000,
			"bonzo_mask":          1_000_000,
		},
	},
	"emerald": {
		"7": {
			"wither_leggings": 5_500_000,
			"wither_cloak":    4_000_000,
			"wither_blood":    2_000_000,
			"wither_helmet":   3_500_000,
			"soul_eater_1":    500_000,
			"wither_boots":    2_000_000,
			"wither_catalyst": 500_000,
		},
		"6": {
			"necromancer_lord_helmet": 1_500_000,
			"sadan_brooch":            1_000_000,
			"necromancer_lord_boots":  1_000_000,
		},
		"5": {
			"aote_stone":              4_500_000,
			"shawdow_assassin_helmet": 1_500_000,
			"fuming_potato_book":      500_000,
			"shadow_assassin_boots":   1_000_000,
			"legion_1":                500_000,
		},
		"4": {
			"thorns_boots":       3_000_000,
			"fuming_potato_book": 500_000,
			// "spirit_pet":       4_500_000,
			"spirit_decoy": 500_000,
		},
		"3": {
			"adaptive_chestplate": 1_500_000,
			"fuming_potato_book":  500_000,
			"adaptive_leggings":   500_000,
		},
		"2": {
			"stone_blade":        1_000_000,
			"fuming_potato_book": 750_000,
		},
		"1": {
			"bonzo_staff":        2_000_000,
			"fuming_potato_book": 1_000_000,
			"bonzo_mask":         1_000_000,
		},
	},
	"diamond": {
		"7": {
			"wither_helmet":   3_750_000,
			"soul_eater_1":    750_000,
			"wither_boots":    2_250_000,
			"wither_catalyst": 750_000,
			"precursor_gear":  250_000,
		},
		"6": {
			"necromancer_lord_boots": 1_250_000,
			"swarm_1":                250_000,
			"giant_tooth":            250_000,
		},
		"5": {
			"shadow_assassin_boots": 1_250_000,
			"legion_1":              750_000,
		},
		"4": {
			"spirit_decoy": 500_000,
			"rend_1":       250_000,
			// "spirit_pet": (epic)      750_000,
		},
		"3": {
			"adaptive_helmet": 250_000,
		},
		"1": {
			"bonzo_helmet": 1_000_000,
		},
	},
	"gold": {
		"7": {
			"wither_boots":    2_400_000,
			"wither_catalyst": 900_000,
			"precursor_gear":  400_000,
		},
		"6": {
			"swarm_1":     400_000,
			"giant_tooth": 400_000,
		},
		"5": {
			"overload_1": 150_000,
			"dark_orb":   150_000,
		},
		"4": {
			"rend_1": 400_000,
		},
	},
}
