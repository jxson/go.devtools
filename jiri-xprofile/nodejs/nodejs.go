// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodejs

import (
	"flag"
	"fmt"
	"path/filepath"
	"runtime"

	"v.io/jiri/lib/profiles"
	"v.io/jiri/lib/tool"
)

const (
	profileName    = "nodejs"
	profileVersion = "1"
	nodeVersion    = "node-v0.10.24"
)

func init() {
	profiles.Register(profileName, &Manager{})
}

type Manager struct {
	root                    string
	nodeRoot                string
	nodeSrcDir, nodeInstDir string
}

func (Manager) Name() string {
	return profileName
}

func (m Manager) String() string {
	return fmt.Sprintf("%s version:%s root:%s", profileName, profileVersion, m.root)
}

func (m Manager) Root() string {
	return m.root
}

func (m *Manager) SetRoot(root string) {
	m.root = root
	m.nodeRoot = filepath.Join(m.root, "profiles", "cout")
	m.nodeSrcDir = filepath.Join(m.root, "profiles", "csrc", nodeVersion)
}

func (m *Manager) AddFlags(flags *flag.FlagSet, action profiles.Action) {
}

func (m *Manager) initForTarget(target profiles.Target) {
	targetDir := profiles.TargetSpecificDirname(target, true)
	m.nodeInstDir = filepath.Join(m.nodeRoot, "node", targetDir)
	// maybe install softlink here..
}

func (m *Manager) Install(ctx *tool.Context, target profiles.Target) error {
	target.Version = profileVersion
	if target.CrossCompiling() {
		return fmt.Errorf("the %q profile does not support cross compilation to %v", profileName, target)
	}
	if err := m.installNode(ctx, target); err != nil {
		return err
	}
	profiles.InstallProfile(profileName, m.nodeRoot)
	return profiles.AddProfileTarget(profileName, target)
}

func (m *Manager) Uninstall(ctx *tool.Context, target profiles.Target) error {
	target.Version = profileVersion
	m.initForTarget(target)
	if err := ctx.Run().RemoveAll(m.nodeInstDir); err != nil {
		return err
	}
	profiles.RemoveProfileTarget(profileName, target)
	return nil
}

func (m *Manager) Update(ctx *tool.Context, target profiles.Target) error {
	target.Version = profileVersion
	m.initForTarget(target)
	return profiles.ErrNoIncrementalUpdate
}

func (m *Manager) installNode(ctx *tool.Context, target profiles.Target) error {
	switch target.OS {
	case "darwin":
	case "linux":
		if err := profiles.InstallPackages(ctx, []string{"g++"}); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%q is not supported", target.OS)

	}
	// Build and install NodeJS.
	installNodeFn := func() error {
		if err := ctx.Run().Chdir(m.nodeSrcDir); err != nil {
			return err
		}
		if err := profiles.RunCommand(ctx, "./configure", []string{fmt.Sprintf("--prefix=%v", m.nodeInstDir)}, nil); err != nil {
			return err
		}
		if err := profiles.RunCommand(ctx, "make", []string{fmt.Sprintf("-j%d", runtime.NumCPU())}, nil); err != nil {
			return err
		}
		if err := profiles.RunCommand(ctx, "make", []string{"install"}, nil); err != nil {
			return err
		}
		return nil
	}
	return profiles.AtomicAction(ctx, installNodeFn, m.nodeInstDir, "Build and install node.js")
}
