// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package flutter_profile

// * Flutter
//     * Flutter engine (osx only)
//       * Depot tools


// git@github.com:flutter/flutter.git
// git@github.com:flutter/engine.git
// https://chromium.googlesource.com/chromium/tools/depot_tools.git

type VersionSpec struct {
	flutterVersion string
	flutterEngineVersion string
	depotToolsVersion string
}

type Manager struct {
	profileInstaller, profileName, qualifiedName string
	flutterDir                                   jiri.RelPath
	flutterEngineDir                             jiri.RelPath
	depotToolsDir                                jiri.RelPath
	versionInfo                                  *profiles.VersionInfo
	versionSpec                                  VersionSpec
}

func Register(installer, profile string) {
	m := &Manager{
		profileInstaller: installer,
		profileName:      profile,
		qualifiedName:    profiles.QualifiedProfileName(installer, profile),
		versionInfo: profiles.NewVersionInfo(profile, map[string]interface{}{
			"1": &versionSpec{
				flutterVersion "2fe456bf957d5df792b716599aa6f21d98997228",
				flutterEngineVersion "98cc27d0f31f1615231e09dc3d490bf00c1b5a4a",
				depotToolsVersion "3ce9b4446dec24fc18bbcdc3e3fde4cc2e27199d",
			},
		}, "1"),
	}
	profilesmanager.Register(m)
}

func (m Manager) Name() string {
	return m.profileName
}

func (m Manager) Installer() string {
	return m.profileInstaller
}

func (m Manager) String() string {
	return fmt.Sprintf("%s[%s]", m.qualifiedName, m.versionInfo.Default())
}

func (m Manager) VersionInfo() *profiles.VersionInfo {
	return m.versionInfo
}

func (m Manager) Info() string {
	return `
The flutter profile provides support for the Flutter mobile framework.`
}

func (m *Manager) AddFlags(flags *flag.FlagSet, action profiles.Action) {}

func (m *Manager) initForTarget(root jiri.RelPath, target profiles.Target) error {
	if err := m.versionInfo.Lookup(target.Version(), &m.version); err != nil {
		return err
	}


	// m.installDir = root.Join("terraform")
	// TODO set dirs here.


	return nil
}

func (m *Manager) Install(jirix *jiri.X, pdb *profiles.DB, root jiri.RelPath, target profiles.Target) error {
	if err := m.initForTarget(root, target); err != nil {
		return err
	}

	if target.CrossCompiling() {
		return fmt.Errorf("the %q profile does not support cross compilation to %v", m.qualifiedName, target)
	}

	if err := m.installAll(jirix, target); err != nil {
		return err
	}

	target.Env.Vars = envvar.MergeSlices(target.Env.Vars, []string{
		"PATH=" + m.installDir.Symbolic(),
	})

	target.InstallationDir = string(m.installDir)
	pdb.InstallProfile(m.profileInstaller, m.profileName, string(m.installDir))
	return pdb.AddProfileTarget(m.profileInstaller, m.profileName, target)
}

func (m *Manager) Uninstall(jirix *jiri.X, pdb *profiles.DB, root jiri.RelPath, target profiles.Target) error {
	if err := m.initForTarget(root, target); err != nil {
		return err
	}
	if err := jirix.NewSeq().RemoveAll(m.installDir.Abs(jirix)).Done(); err != nil {
		return err
	}
	pdb.RemoveProfileTarget(m.profileInstaller, m.profileName, target)
	return nil
}

func (m *Manager) installAll(jirix *jiri.X, target profiles.Target) error {
	tmpDir, err := jirix.NewSeq().TempDir("", "")
	if err != nil {
		return err
	}
	defer jirix.NewSeq().RemoveAll(tmpDir)

	fn := func() error {
		return jirix.NewSeq().
			Call(func() error { return m.installFlutter(jirix, m.flutterDir.Abs(jirix)) }, "Install Flutter")
	}

	return profilesutil.AtomicAction(jirix, fn, m.installDir.Abs(jirix), "Install Terraform")
}

func (m *Manager) installFlutter(jirix *jiri.X, dir string) error {
	fn := func() error {
		return jirix.NewSeq().

	}

	return profilesutil.AtomicAction(jirix, fn, dir, "Install Flutter.")
}
