package memory

import (
	"reflect"
	"testing"

	"github.com/grafana/metrictank/expr/tagquery"
	"github.com/grafana/metrictank/schema"
	"github.com/grafana/metrictank/util"
)

// enableMetaTagSupport enables meta tag support and also tag support
// (because tag-support required to use meta tags)
// it returns a function which resets both setttings to what they were
// before enableMetaTagSupport was called
func enableMetaTagSupport() func() {
	_tagSupport := TagSupport
	_metaTagSupport := MetaTagSupport
	TagSupport = true
	MetaTagSupport = true
	tagquery.MetaTagSupport = true
	return func() {
		TagSupport = _tagSupport
		MetaTagSupport = _metaTagSupport
		tagquery.MetaTagSupport = _metaTagSupport
	}
}

// disableMetaTagSupport disables the meta-tag-support feature flag, it does not
// modify the tag-support feature flag
// it returns a function to reset the meta-tag-support to the previous setting
func disableMetaTagSupport() func() {
	_metaTagSupport := MetaTagSupport
	MetaTagSupport = false
	tagquery.MetaTagSupport = false
	return func() {
		MetaTagSupport = _metaTagSupport
		tagquery.MetaTagSupport = _metaTagSupport
	}
}

func generateMetaRecords(t *testing.T, metaTags, tags [][]string) []tagquery.MetaTagRecord {
	t.Helper()
	if len(metaTags) != len(tags) {
		t.Fatalf("Invalid params to generateMetaRecords, need same number of metaTags and tags")
	}

	res := make([]tagquery.MetaTagRecord, len(metaTags))
	var err error
	for i := 0; i < len(res); i++ {
		res[i], err = tagquery.ParseMetaTagRecord(metaTags[i], tags[i])
		if err != nil {
			t.Fatalf("Invalid meta tags / tags (%q/%q): %s", metaTags[i], tags[i], err.Error())
		}
	}

	return res
}

func TestInsertSimpleMetaTagRecord(t *testing.T) {
	reset := enableMetaTagSupport()
	defer reset()

	metaTagRecords := newMetaTagRecords()
	recordToInsert := generateMetaRecords(t, [][]string{{"metaTag1=abc", "anotherTag=theValue"}}, [][]string{{"metricTag!=a", "match=~this"}})[0]

	_, record, oldId, oldRecord, err := metaTagRecords.upsert(recordToInsert)
	if err != nil {
		t.Fatalf("Unexpected error on meta tag record upsert: %q", err)
	}
	if record == nil {
		t.Fatalf("Record was expected to not be nil, but it was")
	}
	if oldId != 0 {
		t.Fatalf("Old id was expected to be 0, but it was %d", oldId)
	}
	if oldRecord != nil {
		t.Fatalf("OldRecord was expected to be nil, but it was %+v", oldRecord)
	}
	if len(metaTagRecords.records) != 1 {
		t.Fatalf("metaTagRecords was expected to have 1 entry, but it had %d", len(metaTagRecords.records))
	}

	_, ok := metaTagRecords.records[recordId(record.HashExpressions())]
	if !ok {
		t.Fatalf("We expected the record to be found at the index of its hash, but it wasn't")
	}

	if !recordToInsert.Equals(record) {
		t.Fatalf("Inserted meta tag record has unexpectedly been modified")
	}
}

func TestUpdateExistingMetaTagRecord(t *testing.T) {
	reset := enableMetaTagSupport()
	defer reset()

	// the third meta record is going to replace the first meta record because it has the same tag queries
	records := generateMetaRecords(t,
		[][]string{{"metaTag1=value1"}, {"metaTag1=value1"}, {"metaTag1=value2"}},
		[][]string{{"tag1=~a", "tag2=~b"}, {"tag1=~c", "tag2=~d"}, {"tag1=~a", "tag2=~b"}},
	)

	metaTagRecords := newMetaTagRecords()
	metaTagRecords.upsert(records[0])
	metaTagRecords.upsert(records[1])

	if len(metaTagRecords.records) != 2 {
		t.Fatalf("Expected 2 meta tag records, but there were %d", len(metaTagRecords.records))
	}

	var found1, found2 bool
	var recordIdToUpdate recordId
	for i, record := range metaTagRecords.records {
		if record.Equals(&records[0]) {
			found1 = true
			recordIdToUpdate = i
		} else if record.Equals(&records[1]) {
			found2 = true
		}
	}

	if !(found1 && found2) {
		t.Fatalf("Expected both meta tag records to be found, but at least one wasn't: %t / %t", found1, found2)
	}

	id, record, oldId, oldRecord, err := metaTagRecords.upsert(records[2])
	if err != nil {
		t.Fatalf("Expected no error, but there was one: %q", err)
	}
	if record == nil {
		t.Fatalf("Expected record to not be nil, but it was")
	}
	if recordIdToUpdate != id {
		t.Fatalf("Expected the new id after updating to be %d (the id that got returned when creating the record), but it was %d", recordIdToUpdate, id)
	}
	if oldId != id {
		t.Fatalf("Expected the new id after updating to be %d (same as the old id), but it was %d", oldId, id)
	}
	if oldRecord == nil || !oldRecord.Equals(&records[0]) {
		t.Fatalf("Expected the old record to not be nil, but it was")
	}
	if len(metaTagRecords.records) != 2 {
		t.Fatalf("Expected that there to be 2 meta tag records, but there were %d", len(metaTagRecords.records))
	}

	// the order of the records may have changed again due to sorting by id
	found1, found2 = false, false
	for _, record := range metaTagRecords.records {
		if record.Equals(&records[2]) {
			found1 = true
		}
		if record.Equals(&records[1]) {
			found2 = true
		}
	}

	if !(found1 && found2) {
		t.Fatalf("Expected both meta tag records to be found, but not both were: %t / %t", found1, found2)
	}
}

// we mock the hashing algorithm implementation because we want to be able to
// test a hash collision
type mockHash struct {
	returnValues []uint32
	position     int
}

func (m *mockHash) Sum32() uint32 {
	value := m.returnValues[m.position%len(m.returnValues)]
	m.position = (m.position + 1) % len(m.returnValues)
	return value
}

func (m *mockHash) Write(_ []byte) (n int, err error) {
	return
}

func (m *mockHash) WriteString(_ string) (n int, err error) {
	return
}

func (m *mockHash) Sum(_ []byte) (res []byte) {
	return
}

func (m *mockHash) Reset() {}

func (m *mockHash) Size() (n int) {
	return
}
func (m *mockHash) BlockSize() (n int) {
	return
}

// We set the hash collision window to 3, so up to 3 hash collisions are allowed per hash value
// When more than 3 hash collisions are encountered for one hash value, new records are rejected
func TestHashCollisionsOnInsert(t *testing.T) {
	reset := enableMetaTagSupport()
	defer reset()

	originalCollisionAvoidanceWindow := collisionAvoidanceWindow
	defer func() { collisionAvoidanceWindow = originalCollisionAvoidanceWindow }()
	collisionAvoidanceWindow = 3

	originalHash := tagquery.QueryHash
	defer func() { tagquery.QueryHash = originalHash }()

	tagquery.QueryHash = func() util.StringHash32 {
		return &mockHash{
			returnValues: []uint32{1}, // keep returning 1
		}
	}

	metaTagRecords := newMetaTagRecords()
	records := generateMetaRecords(t,
		[][]string{{"metaTag1=value1"}, {"metaTag2=value2"}, {"metaTag3=value3"}, {"metaTag4=value4"}, {"metaTag3=value4"}},
		[][]string{{"metricTag1=value1"}, {"metricTag2=value2"}, {"metricTag3=value3"}, {"metricTag4=value4"}, {"metricTag3=value3"}},
	)
	for i := 0; i < 3; i++ {
		metaTagRecords.upsert(records[i])
	}
	if len(metaTagRecords.records) != 3 {
		t.Fatalf("Expected 3 meta tag records to be present, but there were %d", len(metaTagRecords.records))
	}

	// adding a 4th record with the same hash but different queries
	id, returnedRecord, oldId, oldRecord, err := metaTagRecords.upsert(records[3])
	if err == nil {
		t.Fatalf("Expected an error to be returned, but there was none")
	}
	if id != 0 {
		t.Fatalf("Expected the returned id to be 0, but it was %d", id)
	}
	if returnedRecord != nil {
		t.Fatalf("Expected returned record point to be nil, but it wasn't")
	}
	if oldId != 0 {
		t.Fatalf("Expected oldId to be 0, but it was %d", oldId)
	}
	if oldRecord != nil {
		t.Fatalf("Expected oldRecord to be nil, but it wasn't")
	}
	if len(metaTagRecords.records) != 3 {
		t.Fatalf("Expected 3 metatag records to be present, but there were %d", len(metaTagRecords.records))
	}

	// updating the third record with the same hash and equal queries, but different meta tags
	id, returnedRecord, oldId, oldRecord, err = metaTagRecords.upsert(records[4])
	if err != nil {
		t.Fatalf("Expected no error, but there was one: %q", err)
	}
	if id != 3 {
		t.Fatalf("Expected the returned id to be 3, but it was %d", id)
	}

	// check if the returned new record looks as expected
	if !returnedRecord.Equals(&records[4]) {
		t.Fatalf("New record looked different than expected:\nExpected:\n%+v\nGot:\n%+v\n", &records[4], returnedRecord)
	}
	if oldId != 3 {
		t.Fatalf("Expected oldId to be 3, but it was %d", oldId)
	}

	// check if the returned old record looks as expected
	if !oldRecord.Equals(&records[2]) {
		t.Fatalf("Old record looked different than expected:\nExpected:\n%+v\nGot:\n%+v\n", &records[2], oldRecord)
	}
	if len(metaTagRecords.records) != 3 {
		t.Fatalf("Expected 3 meta tag records to be present, but there were %d", len(metaTagRecords.records))
	}
}

func TestDeletingMetaRecord(t *testing.T) {
	reset := enableMetaTagSupport()
	defer reset()

	// Adding 2 meta records
	metaTagRecords := newMetaTagRecords()
	records := generateMetaRecords(t,
		[][]string{{"metaTag1=value1"}, {"metaTag2=value2"}, {}},
		[][]string{{"metricTag1=value1"}, {"metricTag2=value2"}, {"metricTag2=value2"}},
	)

	metaTagRecords.upsert(records[0])
	idOfRecord2, _, _, _, _ := metaTagRecords.upsert(records[1])

	if len(metaTagRecords.records) != 2 {
		t.Fatalf("Expected that 2 meta tag records exist, but there were %d", len(metaTagRecords.records))
	}

	// then we delete one record again
	// upserting a meta tag record with one that has no meta tags results in deletion
	id, returnedRecord, oldId, _, err := metaTagRecords.upsert(records[2])
	if err != nil {
		t.Fatalf("Expected no error, but there was one: %q", err)
	}
	if len(returnedRecord.MetaTags) != 0 {
		t.Fatalf("Expected returned meta tag record to have 0 meta tags, but it had %d", len(returnedRecord.MetaTags))
	}
	if !returnedRecord.Equals(&records[2]) {
		t.Fatalf("Queries of returned record don't match what we expected:\nExpected:\n%+v\nGot:\n%+v\n", records[2].Expressions, returnedRecord.Expressions)
	}
	if oldId != idOfRecord2 {
		t.Fatalf("Expected the oldId to be the id of record2 (%d), but it was %d", idOfRecord2, oldId)
	}
	if len(metaTagRecords.records) != 1 {
		t.Fatalf("Expected that there is 1 meta tag record, but there were %d", len(metaTagRecords.records))
	}
	_, ok := metaTagRecords.records[id]
	if ok {
		t.Fatalf("Expected returned record id to not be present, but it was")
	}
}

func TestAddingMetricsToEmptyEnricher(t *testing.T) {
	enricher := newEnricher()

	mds := []schema.MetricDefinition{
		{
			Name: "one",
			Tags: []string{"a=b", "c=d"},
		}, {
			Name: "two",
		}, {
			Name: "three",
			Tags: []string{},
		}, {
			Name: "four",
			Tags: []string{"c=d", "e=f"},
		},
	}
	for i := range mds {
		mds[i].SetId()
		enricher.addMetric(mds[i])
	}

	// stop waits for the queue to be consumed
	enricher.stop()
	enricher.start()

	if enricher.countMetricsWithMetaTags() != 0 {
		t.Fatalf("Expected that there are no metrics with meta tags, but there were %d", enricher.countMetricsWithMetaTags())
	}
}

func TestAddingDeletingMetricsAndMetaRecordsToEnricher(t *testing.T) {
	enricher := newEnricher()
	acceptEverySecond, err := tagquery.NewQueryFromStrings([]string{"name=~.*[135]$"}, 0)
	if err != nil {
		t.Fatalf("Unexpected error when parsing query: %s", err.Error())
	}
	acceptAll, err := tagquery.NewQueryFromStrings([]string{"name=~.+"}, 0)
	if err != nil {
		t.Fatalf("Unexpected error when parsing query: %s", err.Error())
	}
	acceptNone, err := tagquery.NewQueryFromStrings([]string{"name=nonexistent"}, 0)
	if err != nil {
		t.Fatalf("Unexpected error when parsing query: %s", err.Error())
	}
	enricher.addMetaRecord(recordId(1), acceptEverySecond, nil)
	enricher.addMetaRecord(recordId(2), acceptAll, nil)
	enricher.addMetaRecord(recordId(3), acceptNone, nil)

	testMetrics := []schema.MetricDefinition{
		{Name: "1"},
		{Name: "2"},
		{Name: "3"},
		{Name: "4"},
		{Name: "5"},
	}

	allKeys := make([]schema.Key, len(testMetrics))
	for i := range testMetrics {
		testMetrics[i].SetId()
		allKeys[i] = testMetrics[i].Id.Key
		enricher.addMetric(testMetrics[i])
	}

	flushEnricherQueue := func() {
		// stop and start to process the event queue
		enricher.stop()
		enricher.start()
	}

	// helper to verify returned result
	compareResultToExpected := func(expected []map[recordId]struct{}) {
		flushEnricherQueue()

		for i, metric := range testMetrics {
			records := enricher.enrich(metric.Id.Key)
			if !reflect.DeepEqual(records, expected[i]) {
				t.Fatalf("Unexpected result returned from enrich:\nExpected: %+v\nGot: %+v\n", expected[i], records)
			}
		}
	}

	compareExpectedMetricCount := func(expected int) {
		flushEnricherQueue()

		result := enricher.countMetricsWithMetaTags()
		if result != expected {
			t.Fatalf("Unexpected count of metrics with meta tags. Expected %d, got %d", expected, result)
		}
	}

	compareResultToExpected([]map[recordId]struct{}{
		{recordId(1): {}, recordId(2): {}},
		{recordId(2): {}},
		{recordId(1): {}, recordId(2): {}},
		{recordId(2): {}},
		{recordId(1): {}, recordId(2): {}},
	})
	compareExpectedMetricCount(5)

	record4Keys := []schema.Key{testMetrics[1].Id.Key, testMetrics[3].Id.Key}
	enricher.addMetaRecord(recordId(4), acceptAll, record4Keys)

	compareResultToExpected([]map[recordId]struct{}{
		{recordId(1): {}, recordId(2): {}},
		{recordId(2): {}, recordId(4): {}},
		{recordId(1): {}, recordId(2): {}},
		{recordId(2): {}, recordId(4): {}},
		{recordId(1): {}, recordId(2): {}},
	})
	compareExpectedMetricCount(5)

	enricher.delMetaRecord(recordId(2), allKeys)
	enricher.delMetaRecord(recordId(4), record4Keys)

	compareResultToExpected([]map[recordId]struct{}{
		{recordId(1): {}},
		nil,
		{recordId(1): {}},
		nil,
		{recordId(1): {}},
	})
	compareExpectedMetricCount(3)

	enricher.delMetric(&testMetrics[4])

	compareResultToExpected([]map[recordId]struct{}{
		{recordId(1): {}},
		nil,
		{recordId(1): {}},
		nil,
		nil,
	})
	compareExpectedMetricCount(2)

	enricher.delMetaRecord(recordId(1), []schema.Key{testMetrics[0].Id.Key, testMetrics[2].Id.Key, testMetrics[4].Id.Key})
	enricher.delMetaRecord(recordId(3), nil)

	compareResultToExpected([]map[recordId]struct{}{
		nil,
		nil,
		nil,
		nil,
		nil,
	})
	compareExpectedMetricCount(0)

	for i := range testMetrics {
		enricher.delMetric(&testMetrics[i])
	}

	compareResultToExpected([]map[recordId]struct{}{
		nil,
		nil,
		nil,
		nil,
		nil,
	})
	compareExpectedMetricCount(0)
}
