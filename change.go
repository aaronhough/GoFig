package main

import (
	"encoding/json"
	"errors"
	"fmt"
)

// <---------------------- Change ------------------------------------>

type Command int

const (
	MigratorUnknown Command = iota
	MigratorUpdate
	MigratorSet
	MigratorAdd
	MigratorDelete
)

type Change struct {
	docPath     string
	before      map[string]any
	patch       map[string]any
	after       map[string]any
	instruction string
	command     Command
	prettyDiff  string
	rollback    string
	errState    error
}

func NewChange(docPath string, before map[string]any, patch map[string]any, command Command, instruction string) *Change {
	c := Change{
		docPath:     docPath,
		before:      before,
		patch:       patch,
		command:     command,
		instruction: instruction,
		errState:    errors.New("Change has not yet been solved."),
	}
	return &c
}

func (c *Change) SolveChange() error {
	c.errState = nil
	err := c.inferAfter()
	if err != nil {
		c.errState = err
		return err
	}
	// if this is a rollback...
	err = c.inferCommand()
	if err != nil {
		c.errState = err
		return err
	}
	err = c.inferPrettyDiff()
	if err != nil {
		c.errState = err
		return err
	}
	err = c.inferRollback()
	if err != nil {
		c.errState = err
		return err
	}
	return nil
}

func (c *Change) commandString() string {
	switch c.command {
	case MigratorUpdate:
		return "update"
	case MigratorSet:
		return "set"
	case MigratorAdd:
		return "add"
	case MigratorDelete:
		return "delete"
	default:
		return "unknown"
	}
}

func (c *Change) inferAfter() error {
	if c.command != MigratorUnknown {
		switch c.command {
		case MigratorSet:
			c.after = c.patch
			return nil
		case MigratorAdd:
			c.after = c.patch
			return nil
		case MigratorDelete:
			c.after = map[string]any{}
			return nil
		}
	}
	if c.before == nil || (c.patch == nil && c.instruction == "") {
		return errors.New("Need before and patch/instruction to infer after.")
	}
	bm, err := json.Marshal(c.before)
	if err != nil {
		return err
	}
	var pm []byte
	if c.patch != nil {
		pm, err = json.Marshal(c.patch)
	} else {
		pm = []byte(c.instruction)
	}
	if err != nil {
		return err
	}
	after, err := applyDiffPatch(bm, pm)
	if err != nil {
		return err
	}
	var ua map[string]any
	json.Unmarshal(after, &ua)
	c.after = ua
	return nil
}

func (c *Change) inferCommand() error {
	// this is only really needed for rollbacks
	if c.command != MigratorUnknown {
		return nil
	}

	if c.after == nil {
		return errors.New("Need after value to infer command.")
	}

	// {}->{...}/{...}->{...} are set... {...}->{} is delete
	if len(c.after) > 0 {
		c.command = MigratorSet
	} else {
		c.command = MigratorDelete
	}

	return nil
}

func (c *Change) inferRollback() error {
	if c.before == nil || c.after == nil {
		return errors.New("Need before and after value to infer rollback.")
	}
	a, err := json.Marshal(c.after)
	if err != nil {
		return err
	}
	b, err := json.Marshal(c.before)
	if err != nil {
		return err
	}
	r, err := getDiffPatch(a, b)
	if err != nil {
		return err
	}
	c.rollback = string(r)
	return nil
}

func (c *Change) inferPrettyDiff() error {

	if c.before == nil || c.after == nil {
		return errors.New("Need before and after value to infer pretty diff.")
	}

	s, err := prettydiff(c.before, c.after)
	if err != nil {
		return err
	}

	c.prettyDiff = s
	return nil
}

func (c *Change) Present() {
	fmt.Println(c.docPath)
	if c.errState != nil {
		fmt.Println("< ERROR STATE... cannot execute changes. >")
		fmt.Println(c.errState.Error())
		return
	}
	fmt.Println(c.prettyDiff)
}

func (c *Change) pushChange(database Firestore) error {
	switch c.command {
	case MigratorUpdate:
		return database.UpdateDoc(c.docPath, c.patch)
	case MigratorSet:
		return database.SetDoc(c.docPath, c.patch)
	case MigratorAdd:
		return database.SetDoc(c.docPath, c.patch)
	default:
		return database.DeleteDoc(c.docPath)
	}
}
