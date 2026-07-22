package ingest

import "strconv"

// addStr, addInt, addFloat and addBool add a property to props only when val
// is present and, for the typed variants, parses cleanly. Empty or
// unparseable CSV cells are simply omitted rather than stored as zero values,
// since Cypher has no useful "null but present" property state.

func addStr(props map[string]any, key, val string) {
	if val != "" {
		props[key] = val
	}
}

func addInt(props map[string]any, key, val string) {
	if val == "" {
		return
	}
	if n, err := strconv.ParseInt(val, 10, 64); err == nil {
		props[key] = n
	}
}

func addFloat(props map[string]any, key, val string) {
	if val == "" {
		return
	}
	if n, err := strconv.ParseFloat(val, 64); err == nil {
		props[key] = n
	}
}

func addBool(props map[string]any, key, val string) {
	switch val {
	case "t", "true", "T", "True":
		props[key] = true
	case "f", "false", "F", "False":
		props[key] = false
	}
}

func equipmentProps(row map[string]string) map[string]any {
	props := map[string]any{}
	addStr(props, "unique_id", row["unique_id"])
	addStr(props, "asset_id", row["asset_id"])
	addStr(props, "brand", row["brand"])
	addStr(props, "model", row["model"])
	addInt(props, "yom", row["yom"])
	addInt(props, "voltage", row["voltage"])
	addStr(props, "contract", row["contract"])
	addStr(props, "serial_no", row["serial_no"])
	addStr(props, "recommendation_status", row["recommendation_status"])
	addBool(props, "dismiss", row["dismiss"])
	addStr(props, "recommended_on", row["recommended_on"])
	addStr(props, "inspected_on", row["inpected_on"])
	addStr(props, "created_on", row["created_on"])
	addStr(props, "modified_on", row["modified_on"])
	addInt(props, "c_year", row["c_year"])
	addStr(props, "display_color", row["display_color"])
	return props
}

func inspectionProps(row map[string]string) map[string]any {
	props := map[string]any{}
	addStr(props, "unique_id", row["equipment_asset_unique_id"])
	addInt(props, "age_at_correction", row["age_at_correction"])
	addStr(props, "date_of_reporting", row["date_of_reporting"])
	addStr(props, "date_of_averting", row["date_of_averting"])
	addFloat(props, "qmax_value", row["qmax_value"])
	addFloat(props, "rep_rate", row["rep_rate"])
	addStr(props, "created_on", row["created_on"])
	addStr(props, "modified_on", row["modified_on"])
	addStr(props, "brand_model", row["brand_model"])
	addInt(props, "c_year", row["c_year"])
	addInt(props, "yom", row["yom"])
	addStr(props, "contract", row["contract"])
	return props
}
