// Copyright © 2016 Asteris, LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package file

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"

	"github.com/asteris-llc/converge/resource"
)

const defaultPermissions = os.FileMode(0750)

const defaultState = "present"

var validStates = []string{"present", "absent"}

const defaultType = "file"

var validFileTypes = []string{"directory", "file"}
var validLinkTypes = []string{"hardlink", "symlink"}

type File struct {
	Destination string
	State       string
	Type        string
	Target      string
	Force       bool
	FileMode    os.FileMode
	User        string
	Group       string
	Content     string
}

func (f *File) Apply() error {
	return nil
}

// Check File settings
func (f *File) Check() (resource.TaskStatus, error) {
	status := &resource.Status{Status: f.Destination}
	var actual *File
	// Get information about the current file
	stat, err := os.Lstat(f.Destination) //link aware

	if os.IsNotExist(err) { //file not found
		switch f.State {
		case "absent": // if "absent" is set and the file doesn't exist, return with no changes
			status.WillChange = false
			status.WarningLevel = resource.StatusNoChange
			return status, nil
		case "present": //file doesn't exist, we need to create it
			actual = &File{Destination: f.Destination, State: "absent"}
			status.WillChange = true
			status.WarningLevel = resource.StatusWillChange
		}
	} else { //file exists
		actual = &File{Destination: f.Destination, State: "present"}
		err = GetFileInfo(actual, stat)
		if err != nil {
			status.WarningLevel = resource.StatusFatal
			return status, fmt.Errorf("unable to get file info for %s: %s", f.Destination, err)
		}
	}
	f.diffFile(actual, status)
	fmt.Println(status.Differences)
	return status, nil
}

func (f *File) Validate() error {
	var err error
	if f.Destination == "" {
		return fmt.Errorf("file requires a destination parameter")
	}

	err = f.validateState()
	if err != nil {
		return err
	}

	err = f.validateType()
	if err != nil {
		return err
	}

	// links should have a target
	err = f.validateTarget()
	if err != nil {
		return err
	}

	err = f.validateUser()
	if err != nil {
		return err
	}

	err = f.validateGroup()
	if err != nil {
		return err
	}

	return err
}

// Validate the state or set default value
func (f *File) validateState() error {
	var err error

	switch f.State {
	case "": //nothing set, use default
		f.State = defaultState
		return nil
	default:
		for _, s := range validStates {
			if f.State == s {
				return nil
			}
		}
		return fmt.Errorf("state should be one of %s, got %q", strings.Join(validStates, ", "), f.State)
	}
	return err
}

// Validate the type or set default value
func (f *File) validateType() error {
	var allTypes []string
	allTypes = append(allTypes, validFileTypes...)
	allTypes = append(allTypes, validLinkTypes...)
	switch f.Type {
	case "": //use default if not set
		f.Type = defaultType
		return nil
	default:
		for _, t := range allTypes {
			if f.Type == t {
				return nil
			}
		}
		return fmt.Errorf("type should be one of %s, got %q", strings.Join(allTypes, ", "), f.Type)
	}
	return nil

}

// A target needs to be set if you are creating a link
func (f *File) validateTarget() error {

	switch f.Target {
	case "":
		if f.Type == "symlink" || f.Type == "hardlink" {
			return fmt.Errorf("must define a target if you are using a %q", f.Type)
		} else {
			return nil
		}
	default:
		// is target set for a file or directory type?
		if f.Type == "symlink" || f.Type == "hardlink" {
			return nil
		} else {
			return fmt.Errorf("cannot define target on a type of %q: target: %q", f.Type, f.Target)
		}
	}
	return fmt.Errorf("unknown combination of type %q and target %q", f.Type, f.Target)
}

func (f *File) validateUser() error {
	if f.User == "" {
		u, err := user.LookupId(strconv.Itoa(os.Geteuid()))
		if err != nil {
			return fmt.Errorf("unable to set default username %s", err)
		}
		f.User = u.Username
	}
	return nil
}

func (f *File) validateGroup() error {
	if f.Group == "" {
		g, err := user.LookupGroupId(strconv.Itoa(os.Getegid()))
		if err != nil {
			return fmt.Errorf("unable to set default group %s", err)
		}
		f.Group = g.Name
	}
	return nil
}

// Populates a File struct with data from a file on the system
func GetFileInfo(f *File, stat os.FileInfo) error {
	var err error

	if f.Destination == "" {
		f.Destination = stat.Name()
	}

	if f.State == "" {
		f.State = "present"
	}

	f.Type, err = FileType(stat)
	if err != nil {
		return fmt.Errorf("error determining type of %s : %s", f.Destination, err)
	}

	// follow symlinks
	if f.Type == "symlink" {
		f.Target, err = os.Readlink(f.Destination)
		if err != nil {
			return fmt.Errorf("error determining target of symlink %s : %s", f.Destination, err)
		}
	}

	f.FileMode = stat.Mode() & os.ModePerm //strip out higher order bits from perms

	f.User, err = FileOwner(stat)
	if err != nil {
		return fmt.Errorf("error determining owner of %s : %s", f.Destination, err)
	}

	f.Group, err = FileGroup(stat)
	if err != nil {
		return fmt.Errorf("error determining group of %s : %s", f.Destination, err)
	}
	return err
}

// Compute the difference between desired and actual state
func (desired *File) diffFile(actual *File, status *resource.Status) {
	var willChange bool

	if desired.State != actual.State {
		willChange = true
		status.AddDifference("state", actual.State, desired.State, "")
	}

	if desired.Type != actual.Type {
		willChange = true
		status.AddDifference("type", actual.Type, desired.Type, "")
	}

	if desired.Target != actual.Target {
		willChange = true
		status.AddDifference("target", actual.Target, desired.Target, "")
	}

	if desired.FileMode != actual.FileMode {
		willChange = true
		status.AddDifference("permissions", actual.FileMode.String(), desired.FileMode.String(), "")
	}

	if desired.User != actual.User {
		willChange = true
		status.AddDifference("user", actual.User, desired.User, "")
	}

	if desired.Group != actual.Group {
		willChange = true
		status.AddDifference("group", actual.Group, desired.Group, "")
	}

	if willChange {
		status.WillChange = true
		status.WarningLevel = resource.StatusWillChange
	}
}