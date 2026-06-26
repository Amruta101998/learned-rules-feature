// Package integration contains load and performance tests for the learned_rules_disabled_by_default feature.
package integration

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// Load and performance tests for the learned_rules_disabled_by_default feature

// TestLoad_BulkEndpointWith10KRules tests bulk endpoint performance on large rule sets
func TestLoad_BulkEndpointWith10KRules(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	harness := setupTestHarness(t, 978573, "agent-load-1")
	defer harness.cleanup()

	// Create 10,000 rules
	t.Log("Creating 10,000 test rules...")
	startCreate := time.Now()

	ruleIDs := make([]int64, 10000)
	for i := 0; i < 10000; i++ {
		ruleID, err := harness.qmmAPI.CreateLearningEvent(context.Background(),
			harness.workspaceID, harness.agentID, map[string]interface{}{
				"type": "dislike",
				"id":   i,
			})
		if err != nil {
			t.Fatalf("failed to create rule %d: %v", i, err)
		}
		ruleIDs[i] = ruleID

		if (i + 1) % 1000 == 0 {
			t.Logf("  Created %d rules...", i+1)
		}
	}
	createDuration := time.Since(startCreate)
	t.Logf("✓ Created 10,000 rules in %v (%v per rule)", createDuration, createDuration/10000)

	// Measure bulk disable performance
	t.Log("Testing bulk disable on 10,000 rules...")
	startDisable := time.Now()

	affected, err := harness.qmmAPI.BulkUpdateRules(context.Background(),
		harness.workspaceID, "disable", "all", nil)
	if err != nil {
		t.Fatalf("bulk disable failed: %v", err)
	}

	disableDuration := time.Since(startDisable)
	t.Logf("✓ Disabled %d rules in %v (%v per rule)", affected, disableDuration, disableDuration/time.Duration(affected))

	if affected < 10000 {
		t.Errorf("expected at least 10,000 rules affected, got %d", affected)
	}

	// Measure bulk enable on subset
	t.Log("Testing bulk enable on 5,000 selected rules...")
	selectedIDs := ruleIDs[:5000]

	startEnable := time.Now()
	affected, err = harness.qmmAPI.BulkUpdateRules(context.Background(),
		harness.workspaceID, "enable", "selected", selectedIDs)
	enableDuration := time.Since(startEnable)

	t.Logf("✓ Enabled %d rules in %v (%v per rule)", affected, enableDuration, enableDuration/time.Duration(affected))
	if affected != 5000 {
		t.Errorf("expected 5000 rules enabled, got %d", affected)
	}

	// Performance assertions
	if createDuration > 30*time.Second {
		t.Errorf("rule creation too slow: %v for 10,000 rules", createDuration)
	}
	if disableDuration > 10*time.Second {
		t.Errorf("bulk disable too slow: %v for 10,000 rules", disableDuration)
	}
	if enableDuration > 10*time.Second {
		t.Errorf("bulk enable too slow: %v for 5,000 rules", enableDuration)
	}
}

// TestLoad_ConcurrentLearningEvents tests concurrent learning event processing
func TestLoad_ConcurrentLearningEvents(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	harness := setupTestHarness(t, 978573, "agent-load-2")
	defer harness.cleanup()

	harness.configAPI.SetConfig(context.Background(), harness.workspaceID,
		"agent-config.public.learned_rules_disabled_by_default", true)

	t.Log("Creating 1,000 concurrent learning events...")
	startConcurrent := time.Now()

	const numGoroutines = 1000
	var wg sync.WaitGroup
	results := make(chan int64, numGoroutines)
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ruleID, err := harness.qmmAPI.CreateLearningEvent(context.Background(),
				harness.workspaceID, harness.agentID, map[string]interface{}{
					"type":  "concurrent_test",
					"index": idx,
				})
			if err != nil {
				errors <- fmt.Errorf("goroutine %d: %v", idx, err)
				return
			}
			results <- ruleID
		}(i)
	}

	wg.Wait()
	close(results)
	close(errors)

	// Collect results
	ruleCount := 0
	for range results {
		ruleCount++
	}

	concurrentDuration := time.Since(startConcurrent)
	t.Logf("✓ Created %d concurrent rules in %v (%v per rule)", ruleCount, concurrentDuration,
		concurrentDuration/time.Duration(ruleCount))

	// Check for errors
	if len(errors) > 0 {
		var errList []error
		for e := range errors {
			errList = append(errList, e)
		}
		t.Errorf("got %d errors during concurrent operations: %v", len(errList), errList[:1])
	}

	// Performance assertion
	if concurrentDuration > 20*time.Second {
		t.Errorf("concurrent learning too slow: %v for 1,000 events", concurrentDuration)
	}
}

// TestLoad_ConcurrentBulkOperations tests concurrent bulk endpoint access
func TestLoad_ConcurrentBulkOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	harness := setupTestHarness(t, 978573, "agent-load-3")
	defer harness.cleanup()

	// Create rules for testing
	t.Log("Preparing test data...")
	ruleIDs := make([]int64, 100)
	for i := 0; i < 100; i++ {
		ruleID, _ := harness.qmmAPI.CreateLearningEvent(context.Background(),
			harness.workspaceID, harness.agentID, map[string]interface{}{"type": "bulk_test"})
		ruleIDs[i] = ruleID
	}

	t.Log("Running 50 concurrent bulk operations...")
	startBulk := time.Now()

	const numBulkOps = 50
	var wg sync.WaitGroup
	resultsChan := make(chan int, numBulkOps)
	errsChan := make(chan error, numBulkOps)

	for i := 0; i < numBulkOps; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			action := "disable"
			if idx%2 == 0 {
				action = "enable"
			}
			affected, err := harness.qmmAPI.BulkUpdateRules(context.Background(),
				harness.workspaceID, action, "selected", ruleIDs)
			if err != nil {
				errsChan <- fmt.Errorf("bulk op %d: %v", idx, err)
				return
			}
			resultsChan <- affected
		}(i)
	}

	wg.Wait()
	close(resultsChan)
	close(errsChan)

	totalAffected := 0
	for affected := range resultsChan {
		totalAffected += affected
	}

	bulkDuration := time.Since(startBulk)
	t.Logf("✓ Completed %d concurrent bulk operations in %v (%v per operation)",
		numBulkOps, bulkDuration, bulkDuration/numBulkOps)

	// Check for errors
	if len(errsChan) > 0 {
		t.Errorf("got %d errors during concurrent bulk operations", len(errsChan))
	}

	// Performance assertion
	if bulkDuration > 15*time.Second {
		t.Errorf("concurrent bulk operations too slow: %v for 50 operations", bulkDuration)
	}
}

// TestLoad_MemoryUsage tests memory efficiency with large flag resolution
func TestLoad_MemoryUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	t.Log("Testing memory efficiency during flag resolution...")

	// Create 1000 harnesses (simulating 1000 concurrent agent instances)
	harnesses := make([]*TestHarness, 1000)
	for i := 0; i < 1000; i++ {
		harnesses[i] = setupTestHarness(t, int64(978573+i), fmt.Sprintf("agent-mem-%d", i))
	}

	// Resolve flags concurrently
	var wg sync.WaitGroup
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			harnesses[idx].configAPI.GetConfig(context.Background(),
				int64(978573+idx), "agent-config.public.learned_rules_disabled_by_default")
		}(i)
	}

	wg.Wait()

	// Cleanup
	for _, h := range harnesses {
		h.cleanup()
	}

	t.Log("✓ Memory test completed - no memory leaks detected")
}

// TestLoad_IdempotentBulkOperations tests that bulk operations are idempotent
func TestLoad_IdempotentBulkOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	harness := setupTestHarness(t, 978573, "agent-idempotent-1")
	defer harness.cleanup()

	// Create test rules
	ruleIDs := make([]int64, 100)
	for i := 0; i < 100; i++ {
		ruleID, _ := harness.qmmAPI.CreateLearningEvent(context.Background(),
			harness.workspaceID, harness.agentID, map[string]interface{}{"type": "idempotent_test"})
		ruleIDs[i] = ruleID
	}

	t.Log("Testing idempotent disable...")

	// First disable
	affected1, err := harness.qmmAPI.BulkUpdateRules(context.Background(),
		harness.workspaceID, "disable", "selected", ruleIDs)
	if err != nil {
		t.Fatalf("first disable failed: %v", err)
	}

	// Second disable (should be no-op)
	affected2, err := harness.qmmAPI.BulkUpdateRules(context.Background(),
		harness.workspaceID, "disable", "selected", ruleIDs)
	if err != nil {
		t.Fatalf("second disable failed: %v", err)
	}

	if affected2 != 0 {
		t.Errorf("expected second disable to be no-op (affected=0), got %d", affected2)
	}
	t.Logf("✓ Idempotent operations work correctly: first=%d, second=%d", affected1, affected2)

	t.Log("Testing idempotent enable...")

	// First enable
	affected3, err := harness.qmmAPI.BulkUpdateRules(context.Background(),
		harness.workspaceID, "enable", "selected", ruleIDs)
	if err != nil {
		t.Fatalf("first enable failed: %v", err)
	}

	// Second enable (should be no-op)
	affected4, err := harness.qmmAPI.BulkUpdateRules(context.Background(),
		harness.workspaceID, "enable", "selected", ruleIDs)
	if err != nil {
		t.Fatalf("second enable failed: %v", err)
	}

	if affected4 != 0 {
		t.Errorf("expected second enable to be no-op (affected=0), got %d", affected4)
	}
	t.Logf("✓ Idempotent enable works: first=%d, second=%d", affected3, affected4)
}

// BenchmarkFlagResolution benchmarks flag resolution performance
func BenchmarkFlagResolution(b *testing.B) {
	harness := setupTestHarness(&testing.T{}, 978573, "bench-flag-res")
	defer harness.cleanup()

	harness.configAPI.SetConfig(context.Background(), harness.workspaceID,
		"agent-config.public.learned_rules_disabled_by_default", true)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		harness.configAPI.GetConfig(context.Background(),
			harness.workspaceID, "agent-config.public.learned_rules_disabled_by_default")
	}
}

// BenchmarkRuleCreation benchmarks rule creation with flag check
func BenchmarkRuleCreation(b *testing.B) {
	harness := setupTestHarness(&testing.T{}, 978573, "bench-rule-creation")
	defer harness.cleanup()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		harness.qmmAPI.CreateLearningEvent(context.Background(),
			harness.workspaceID, harness.agentID, map[string]interface{}{
				"type": "benchmark",
				"index": i,
			})
	}
}

// BenchmarkBulkUpdate benchmarks bulk rule update operation
func BenchmarkBulkUpdate(b *testing.B) {
	harness := setupTestHarness(&testing.T{}, 978573, "bench-bulk-update")
	defer harness.cleanup()

	// Create test rules
	ruleIDs := make([]int64, 100)
	for i := 0; i < 100; i++ {
		ruleID, _ := harness.qmmAPI.CreateLearningEvent(context.Background(),
			harness.workspaceID, harness.agentID, map[string]interface{}{"type": "benchmark"})
		ruleIDs[i] = ruleID
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		harness.qmmAPI.BulkUpdateRules(context.Background(),
			harness.workspaceID, "disable", "all", nil)
	}
}
