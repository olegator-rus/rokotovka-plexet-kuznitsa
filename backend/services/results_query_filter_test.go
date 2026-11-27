package services

import (
	"testing"

	"kuznica/api/domain"
	"kuznica/api/out"
)

func TestQueryLicFiltersFromChips(t *testing.T) {
	chips := []out.Chip{
		{Field: fieldVendor, Value: "kaspersky"},
		{Field: domain.FieldLicenseDuration, Value: "12", Label: "Срок 1 год"},
		{Field: domain.FieldLicenseType, Value: domain.LicensePerpetual, Label: "Бессрочная"},
	}
	fs := queryLicFilters(chips, nil)
	if len(fs) != 2 || fs[0].key != facetDuration || !fs[0].isNum || fs[0].num != 12 || fs[1].key != facetLicType || fs[1].str != domain.LicensePerpetual {
		t.Fatalf("фильтры из чипов: %+v", fs)
	}
	// Фильтр панели по тому же полю важнее чипа запроса.
	if fs := queryLicFilters(chips, []licFilter{{key: facetDuration, op: "eq", str: "24"}}); len(fs) != 1 || fs[0].key != facetLicType {
		t.Fatalf("поле из панели не пропущено: %+v", fs)
	}
}

func TestQuantityFacetLabels(t *testing.T) {
	dev, usr := domain.MetricDevice, domain.MetricUser
	q := quantityMetrics{}
	q.add("10", indexDoc{LicenseMetric: &dev})
	q.add("10", indexDoc{})
	q.add("1", indexDoc{LicenseMetric: &dev})
	labels := map[string]string{}
	q.label(labels)
	if labels["10"] != "10 устройств" || labels["1"] != "1 устройство" {
		t.Fatalf("одна метрика: %v", labels)
	}
	q.add("5", indexDoc{LicenseMetric: &usr})
	q.label(labels)
	if labels["10"] != "10 мест" || labels["5"] != "5 мест" {
		t.Fatalf("разные метрики: %v", labels)
	}
}
