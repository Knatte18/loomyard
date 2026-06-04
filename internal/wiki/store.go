package wiki

import (
	"encoding/json"
	"fmt"
	"os"
)

type Store struct {
	tasks    []Task
	filePath string
}

func NewStore(filePath string) *Store {
	return &Store{
		tasks:    []Task{},
		filePath: filePath,
	}
}

func (s *Store) Load() error {
	if s.filePath == "" {
		s.tasks = []Task{}
		return nil
	}

	content, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			s.tasks = []Task{}
			return nil
		}
		// Silent fallback on any other error
		s.tasks = []Task{}
		return nil
	}

	var tasks []Task
	err = json.Unmarshal(content, &tasks)
	if err != nil {
		// Silent fallback on parse error
		s.tasks = []Task{}
		return nil
	}

	// Normalize DependsOn: set to empty slice if nil
	for i := range tasks {
		if tasks[i].DependsOn == nil {
			tasks[i].DependsOn = []string{}
		}
	}

	s.tasks = tasks
	return nil
}

func (s *Store) Save(wikiPath, relPath string) error {
	content, err := json.MarshalIndent(s.tasks, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal tasks: %w", err)
	}

	err = AtomicWrite(wikiPath, relPath, string(content))
	if err != nil {
		return fmt.Errorf("atomic write: %w", err)
	}

	return nil
}

func (s *Store) Tasks() []Task {
	result := make([]Task, len(s.tasks))
	copy(result, s.tasks)
	return result
}

func (s *Store) slugIndex() map[string]*Task {
	index := make(map[string]*Task)
	for i := range s.tasks {
		index[s.tasks[i].Slug] = &s.tasks[i]
	}
	return index
}

func (s *Store) nextID() int {
	if len(s.tasks) == 0 {
		return 0
	}
	maxID := s.tasks[0].ID
	for _, t := range s.tasks {
		if t.ID > maxID {
			maxID = t.ID
		}
	}
	return maxID + 1
}
