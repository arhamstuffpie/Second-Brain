package repository

import "fmt"

type Container struct {
	Health      HealthRepository
	User        UserRepository
	Models      ModelProfileRepository
	Speakers    SpeakerProfileRepository
	People      PersonRepository
	DensePeople DensePersonAnalysisRepository
	Voice       VoiceRepository
	Video       VideoRepository
}

func NewContainer(db DBTX) (*Container, error) {
	baseRepository, err := newBase(db)
	if err != nil {
		return nil, err
	}

	container := &Container{
		Health:      newHealthRepository(baseRepository),
		User:        newUserRepository(baseRepository),
		Models:      newModelProfileRepository(baseRepository),
		Speakers:    newSpeakerProfileRepository(baseRepository),
		People:      newPersonRepository(baseRepository),
		DensePeople: newDensePersonAnalysisRepository(baseRepository),
		Voice:       newVoiceRepository(baseRepository),
		Video:       newVideoRepository(baseRepository),
	}
	if err := container.Validate(); err != nil {
		return nil, err
	}
	return container, nil
}

func (c *Container) Validate() error {
	if c == nil {
		return fmt.Errorf("repository container is required")
	}
	if c.Health == nil {
		return fmt.Errorf("health repository is required")
	}
	if c.User == nil {
		return fmt.Errorf("user repository is required")
	}
	if c.Models == nil {
		return fmt.Errorf("model profile repository is required")
	}
	if c.Speakers == nil {
		return fmt.Errorf("speaker profile repository is required")
	}
	if c.People == nil {
		return fmt.Errorf("person repository is required")
	}
	if c.DensePeople == nil {
		return fmt.Errorf("dense person analysis repository is required")
	}
	if c.Voice == nil {
		return fmt.Errorf("voice repository is required")
	}
	if c.Video == nil {
		return fmt.Errorf("video repository is required")
	}
	return nil
}
