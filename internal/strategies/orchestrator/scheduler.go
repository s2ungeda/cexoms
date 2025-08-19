package orchestrator

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// ScheduleConfig represents scheduling configuration for a strategy
type ScheduleConfig struct {
	ActiveHours []TimeWindow `yaml:"active_hours"`
	Timezone    string       `yaml:"timezone"`
}

// TimeWindow represents an active time window
type TimeWindow struct {
	Start string   `yaml:"start"` // HH:MM format
	End   string   `yaml:"end"`   // HH:MM format
	Days  []string `yaml:"days"`  // mon, tue, wed, thu, fri, sat, sun
}

// ScheduledStrategy represents a strategy with scheduling
type ScheduledStrategy struct {
	StrategyID   string
	StrategyType StrategyType
	Schedule     ScheduleConfig
	Config       interface{}
	Accounts     []string
	IsActive     bool
	NextStart    *time.Time
	NextStop     *time.Time
}

// Scheduler manages strategy scheduling
type Scheduler struct {
	orchestrator      *Orchestrator
	schedules         map[string]*ScheduledStrategy
	mu                sync.RWMutex
	ctx               context.Context
	cancel            context.CancelFunc
	wg                sync.WaitGroup
	checkInterval     time.Duration
	defaultTimezone   *time.Location
}

// NewScheduler creates a new strategy scheduler
func NewScheduler(orchestrator *Orchestrator) (*Scheduler, error) {
	ctx, cancel := context.WithCancel(context.Background())
	
	// Default to UTC
	defaultTZ, err := time.LoadLocation("UTC")
	if err != nil {
		return nil, fmt.Errorf("failed to load UTC timezone: %w", err)
	}

	return &Scheduler{
		orchestrator:    orchestrator,
		schedules:       make(map[string]*ScheduledStrategy),
		ctx:             ctx,
		cancel:          cancel,
		checkInterval:   1 * time.Minute,
		defaultTimezone: defaultTZ,
	}, nil
}

// AddScheduledStrategy adds a strategy with scheduling
func (s *Scheduler) AddScheduledStrategy(strategyType StrategyType, schedule ScheduleConfig, config interface{}, accounts []string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate schedule
	if err := s.validateSchedule(schedule); err != nil {
		return "", fmt.Errorf("invalid schedule: %w", err)
	}

	// Generate ID
	id := fmt.Sprintf("sched_%s_%d", strategyType, time.Now().UnixNano())

	// Create scheduled strategy
	scheduled := &ScheduledStrategy{
		StrategyID:   "",
		StrategyType: strategyType,
		Schedule:     schedule,
		Config:       config,
		Accounts:     accounts,
		IsActive:     false,
	}

	// Calculate next start/stop times
	s.updateNextTimes(scheduled)

	s.schedules[id] = scheduled

	log.Printf("Added scheduled strategy %s of type %s", id, strategyType)
	return id, nil
}

// RemoveScheduledStrategy removes a scheduled strategy
func (s *Scheduler) RemoveScheduledStrategy(scheduleID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	scheduled, exists := s.schedules[scheduleID]
	if !exists {
		return fmt.Errorf("scheduled strategy not found: %s", scheduleID)
	}

	// Stop if running
	if scheduled.IsActive && scheduled.StrategyID != "" {
		if err := s.orchestrator.StopStrategy(scheduled.StrategyID); err != nil {
			log.Printf("Error stopping strategy %s: %v", scheduled.StrategyID, err)
		}
	}

	delete(s.schedules, scheduleID)
	return nil
}

// Start starts the scheduler
func (s *Scheduler) Start() error {
	s.wg.Add(1)
	go s.run()
	
	log.Println("Strategy scheduler started")
	return nil
}

// Stop stops the scheduler
func (s *Scheduler) Stop() error {
	log.Println("Stopping strategy scheduler...")
	
	s.cancel()
	s.wg.Wait()
	
	// Stop all active scheduled strategies
	s.mu.Lock()
	for _, scheduled := range s.schedules {
		if scheduled.IsActive && scheduled.StrategyID != "" {
			if err := s.orchestrator.StopStrategy(scheduled.StrategyID); err != nil {
				log.Printf("Error stopping scheduled strategy %s: %v", scheduled.StrategyID, err)
			}
		}
	}
	s.mu.Unlock()
	
	log.Println("Strategy scheduler stopped")
	return nil
}

// run is the main scheduler loop
func (s *Scheduler) run() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.checkInterval)
	defer ticker.Stop()

	// Initial check
	s.checkSchedules()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.checkSchedules()
		}
	}
}

// checkSchedules checks all schedules and starts/stops strategies as needed
func (s *Scheduler) checkSchedules() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	for id, scheduled := range s.schedules {
		shouldBeActive := s.shouldBeActive(scheduled, now)

		if shouldBeActive && !scheduled.IsActive {
			// Start strategy
			s.startScheduledStrategy(id, scheduled)
		} else if !shouldBeActive && scheduled.IsActive {
			// Stop strategy
			s.stopScheduledStrategy(id, scheduled)
		}

		// Update next times
		s.updateNextTimes(scheduled)
	}
}

// shouldBeActive checks if a strategy should be active at the given time
func (s *Scheduler) shouldBeActive(scheduled *ScheduledStrategy, now time.Time) bool {
	// Get timezone
	tz := s.getTimezone(scheduled.Schedule.Timezone)
	localTime := now.In(tz)

	// Get current day
	currentDay := s.getDayAbbreviation(localTime.Weekday())

	// Check each time window
	for _, window := range scheduled.Schedule.ActiveHours {
		// Check if current day is in the window
		dayMatch := false
		for _, day := range window.Days {
			if day == currentDay {
				dayMatch = true
				break
			}
		}

		if !dayMatch {
			continue
		}

		// Parse start and end times
		startTime, err := s.parseTimeOfDay(window.Start, localTime)
		if err != nil {
			log.Printf("Error parsing start time %s: %v", window.Start, err)
			continue
		}

		endTime, err := s.parseTimeOfDay(window.End, localTime)
		if err != nil {
			log.Printf("Error parsing end time %s: %v", window.End, err)
			continue
		}

		// Check if current time is within window
		if localTime.After(startTime) && localTime.Before(endTime) {
			return true
		}
	}

	return false
}

// startScheduledStrategy starts a scheduled strategy
func (s *Scheduler) startScheduledStrategy(scheduleID string, scheduled *ScheduledStrategy) {
	strategyID, err := s.orchestrator.StartStrategy(scheduled.StrategyType, scheduled.Config, scheduled.Accounts)
	if err != nil {
		log.Printf("Failed to start scheduled strategy %s: %v", scheduleID, err)
		return
	}

	scheduled.StrategyID = strategyID
	scheduled.IsActive = true

	log.Printf("Started scheduled strategy %s with ID %s", scheduleID, strategyID)
}

// stopScheduledStrategy stops a scheduled strategy
func (s *Scheduler) stopScheduledStrategy(scheduleID string, scheduled *ScheduledStrategy) {
	if scheduled.StrategyID == "" {
		return
	}

	if err := s.orchestrator.StopStrategy(scheduled.StrategyID); err != nil {
		log.Printf("Failed to stop scheduled strategy %s: %v", scheduled.StrategyID, err)
		return
	}

	scheduled.StrategyID = ""
	scheduled.IsActive = false

	log.Printf("Stopped scheduled strategy %s", scheduleID)
}

// updateNextTimes updates the next start and stop times for a scheduled strategy
func (s *Scheduler) updateNextTimes(scheduled *ScheduledStrategy) {
	now := time.Now()
	tz := s.getTimezone(scheduled.Schedule.Timezone)
	
	var nextStart, nextStop *time.Time
	
	// Find next start and stop times
	for i := 0; i < 7; i++ { // Check next 7 days
		checkDate := now.AddDate(0, 0, i).In(tz)
		dayAbbr := s.getDayAbbreviation(checkDate.Weekday())
		
		for _, window := range scheduled.Schedule.ActiveHours {
			// Check if this day is included
			dayIncluded := false
			for _, day := range window.Days {
				if day == dayAbbr {
					dayIncluded = true
					break
				}
			}
			
			if !dayIncluded {
				continue
			}
			
			// Parse times for this day
			startTime, _ := s.parseTimeOfDay(window.Start, checkDate)
			endTime, _ := s.parseTimeOfDay(window.End, checkDate)
			
			// Update next start if this is sooner
			if startTime.After(now) && (nextStart == nil || startTime.Before(*nextStart)) {
				nextStart = &startTime
			}
			
			// Update next stop if we're currently in a window
			if scheduled.IsActive && now.After(startTime) && now.Before(endTime) {
				if nextStop == nil || endTime.Before(*nextStop) {
					nextStop = &endTime
				}
			}
		}
	}
	
	scheduled.NextStart = nextStart
	scheduled.NextStop = nextStop
}

// validateSchedule validates a schedule configuration
func (s *Scheduler) validateSchedule(schedule ScheduleConfig) error {
	if len(schedule.ActiveHours) == 0 {
		return fmt.Errorf("no active hours defined")
	}

	for i, window := range schedule.ActiveHours {
		if window.Start == "" || window.End == "" {
			return fmt.Errorf("window %d: start and end times required", i)
		}

		if len(window.Days) == 0 {
			return fmt.Errorf("window %d: no days specified", i)
		}

		// Validate day abbreviations
		for _, day := range window.Days {
			if !s.isValidDayAbbreviation(day) {
				return fmt.Errorf("window %d: invalid day abbreviation: %s", i, day)
			}
		}
	}

	return nil
}

// Helper functions

func (s *Scheduler) getTimezone(tzName string) *time.Location {
	if tzName == "" {
		return s.defaultTimezone
	}

	tz, err := time.LoadLocation(tzName)
	if err != nil {
		log.Printf("Failed to load timezone %s, using default: %v", tzName, err)
		return s.defaultTimezone
	}

	return tz
}

func (s *Scheduler) parseTimeOfDay(timeStr string, referenceDate time.Time) (time.Time, error) {
	// Parse HH:MM format
	t, err := time.Parse("15:04", timeStr)
	if err != nil {
		return time.Time{}, err
	}

	// Combine with reference date
	return time.Date(
		referenceDate.Year(),
		referenceDate.Month(),
		referenceDate.Day(),
		t.Hour(),
		t.Minute(),
		0, 0,
		referenceDate.Location(),
	), nil
}

func (s *Scheduler) getDayAbbreviation(weekday time.Weekday) string {
	days := []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}
	return days[weekday]
}

func (s *Scheduler) isValidDayAbbreviation(day string) bool {
	validDays := []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}
	for _, valid := range validDays {
		if day == valid {
			return true
		}
	}
	return false
}

// GetScheduledStrategies returns all scheduled strategies
func (s *Scheduler) GetScheduledStrategies() map[string]*ScheduledStrategy {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Create a copy to avoid external modifications
	result := make(map[string]*ScheduledStrategy)
	for k, v := range s.schedules {
		result[k] = v
	}

	return result
}