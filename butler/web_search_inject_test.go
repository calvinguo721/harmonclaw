package butler

import "testing"

func TestShouldTriggerWebSearch_GuangzhouWeather(t *testing.T) {
	if !shouldTriggerWebSearch("search today weather in guangzhou") {
		t.Fatal(`expected true for "search today weather in guangzhou" (contains "search")`)
	}
}
