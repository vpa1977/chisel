package setup_test

import (
	"io/ioutil"
	"os"
	"path/filepath"

	. "gopkg.in/check.v1"

	"github.com/canonical/chisel/internal/setup"
	"github.com/canonical/chisel/internal/testutil"
)

type setupTest struct {
	summary   string
	input     map[string]string
	slices    map[string]*setup.Slice
	release   *setup.Release
	relerror  string
	selslices []setup.SliceKey
	selection *setup.Selection
	selerror  string
}

var setupTests = []setupTest{{
	summary: "Ensure file format is expected",
	input: map[string]string{
		"chisel.yaml": `
			format: foobar
		`,
	},
	relerror: `chisel.yaml: expected format "chisel-v1", got "foobar"`,
}, {
	summary: "Missing archives",
	input: map[string]string{
		"chisel.yaml": `
			format: chisel-v1
		`,
	},
	relerror: `chisel.yaml: no archives defined`,
}, {
	summary: "Multiple archives",
	input: map[string]string{
		"chisel.yaml": `
			format: chisel-v1
			archives: {one: {version: 1}, two: {version: two}}
		`,
	},
	relerror: `chisel.yaml: multiple archives not yet supported`,
}, {
	summary: "Only ubuntu archives for now",
	input: map[string]string{
		"chisel.yaml": `
			format: chisel-v1
			archives: {other: {version: 1}}
		`,
	},
	relerror: `chisel.yaml: only "ubuntu" archives are supported for now`,
}, {
	summary: "Enforce matching filename and package name",
	input: map[string]string{
		"slices/mydir/mypkg.yaml": `
			package: myotherpkg
		`,
	},
	relerror: `slices/mydir/mypkg.yaml: filename and 'package' field \("myotherpkg"\) disagree`,
}, {
	summary: "Simple example",
	input: map[string]string{
		"slices/mydir/mypkg.yaml": `
			package: mypkg
			slices:
				myslice1:
					contents:
						/file/path1:
						/file/path2: {copy: /other/path}
						/file/path3: {symlink: /other/path}
						/file/path4: {text: "content"}
						/file/path5: {mode: 0755, mutable: true}
						/file/path6/: {make: true}
				myslice2:
					essential:
						- mypkg.myslice1
					contents:
						/another/path:
		`,
	},
	release: &setup.Release{
		DefaultArchive: "ubuntu",

		Archives: map[string]*setup.Archive{"ubuntu": {"ubuntu", "22.04", []string{"main", "universe"}}},
		Packages: map[string]*setup.Package{
			"mypkg": {
				Archive: "ubuntu",
				Name:    "mypkg",
				Path:    "slices/mydir/mypkg.yaml",
				Slices: map[string]*setup.Slice{
					"myslice1": {
						Package: "mypkg",
						Name:    "myslice1",
						Contents: map[string]setup.PathInfo{
							"/file/path1":  {Kind: "copy"},
							"/file/path2":  {Kind: "copy", Info: "/other/path"},
							"/file/path3":  {Kind: "symlink", Info: "/other/path"},
							"/file/path4":  {Kind: "text", Info: "content"},
							"/file/path5":  {Kind: "copy", Mode: 0755, Mutable: true},
							"/file/path6/": {Kind: "dir"},
						},
					},
					"myslice2": {
						Package: "mypkg",
						Name:    "myslice2",
						Essential: []setup.SliceKey{
							{"mypkg", "myslice1"},
						},
						Contents: map[string]setup.PathInfo{
							"/another/path": {Kind: "copy"},
						},
					},
				},
			},
		},
	},
}, {
	summary: "Cycles are detected within packages",
	input: map[string]string{
		"slices/mydir/mypkg.yaml": `
			package: mypkg
			slices:
				myslice1:
					essential:
						- mypkg.myslice2
				myslice2:
					essential:
						- mypkg.myslice3
				myslice3:
					essential:
						- mypkg.myslice1
		`,
	},
	relerror: `essential loop detected: mypkg.myslice1, mypkg.myslice2, mypkg.myslice3`,
}, {
	summary: "Cycles are detected across packages",
	input: map[string]string{
		"slices/mydir/mypkg1.yaml": `
			package: mypkg1
			slices:
				myslice:
					essential:
						- mypkg2.myslice
		`,
		"slices/mydir/mypkg2.yaml": `
			package: mypkg2
			slices:
				myslice:
					essential:
						- mypkg3.myslice
		`,
		"slices/mydir/mypkg3.yaml": `
			package: mypkg3
			slices:
				myslice:
					essential:
						- mypkg1.myslice
		`,
	},
	relerror: `essential loop detected: mypkg1.myslice, mypkg2.myslice, mypkg3.myslice`,
}, {
	summary: "Missing package dependency",
	input: map[string]string{
		"slices/mydir/mypkg1.yaml": `
			package: mypkg1
			slices:
				myslice:
					essential:
						- mypkg2.myslice
		`,
	},
	relerror: `mypkg1.myslice requires mypkg2.myslice, but slice is missing`,
}, {
	summary: "Missing slice dependency",
	input: map[string]string{
		"slices/mydir/mypkg.yaml": `
			package: mypkg
			slices:
				myslice1:
					essential:
						- mypkg.myslice2
		`,
	},
	relerror: `mypkg.myslice1 requires mypkg.myslice2, but slice is missing`,
}, {
	summary: "Selection with no dependencies",
	input: map[string]string{
		"slices/mydir/mypkg1.yaml": `
			package: mypkg1
			slices:
				myslice1: {}
				myslice2: {essential: [mypkg2.myslice1]}
		`,
		"slices/mydir/mypkg2.yaml": `
			package: mypkg2
			slices:
				myslice1: {}
				myslice2: {essential: [mypkg1.myslice1]}
		`,
	},
	selslices: []setup.SliceKey{{"mypkg1", "myslice1"}},
	selection: &setup.Selection{
		Slices: []*setup.Slice{{
			Package: "mypkg1",
			Name:    "myslice1",
		}},
	},
}, {
	summary: "Selection with dependencies",
	input: map[string]string{
		"slices/mydir/mypkg1.yaml": `
			package: mypkg1
			slices:
				myslice1: {}
				myslice2: {essential: [mypkg2.myslice1]}
		`,
		"slices/mydir/mypkg2.yaml": `
			package: mypkg2
			slices:
				myslice1: {}
				myslice2: {essential: [mypkg1.myslice1]}
		`,
	},
	selslices: []setup.SliceKey{{"mypkg2", "myslice2"}},
	selection: &setup.Selection{
		Slices: []*setup.Slice{{
			Package: "mypkg1",
			Name:    "myslice1",
		}, {
			Package: "mypkg2",
			Name:    "myslice2",
			Essential: []setup.SliceKey{
				{"mypkg1", "myslice1"},
			},
		}},
	},
}, {
	summary: "Selection with matching paths don't conflict",
	input: map[string]string{
		"slices/mydir/mypkg1.yaml": `
			package: mypkg1
			slices:
				myslice1:
					contents:
						/path1:
						/path2: {text: same}
						/path3: {symlink: /link}
				myslice2:
					contents:
						/path1: {copy: /path1}
						/path2: {text: same}
						/path3: {symlink: /link}
		`,
		"slices/mydir/mypkg2.yaml": `
			package: mypkg2
			slices:
				myslice1:
					contents:
						/path2: {text: same}
						/path3: {symlink: /link}
		`,
	},
	selslices: []setup.SliceKey{{"mypkg1", "myslice1"}, {"mypkg1", "myslice2"}, {"mypkg2", "myslice1"}},
}, {
	summary: "Selection with conflicting paths across slices",
	input: map[string]string{
		"slices/mydir/mypkg1.yaml": `
			package: mypkg1
			slices:
				myslice1:
					contents:
						/path1:
				myslice2:
					contents:
						/path1: {copy: /other}
		`,
	},
	selslices: []setup.SliceKey{{"mypkg1", "myslice1"}, {"mypkg1", "myslice2"}},
	selerror:  "slices mypkg1.myslice1 and mypkg1.myslice2 conflict on /path1",
}, {
	summary: "Selection with conflicting paths across packages",
	input: map[string]string{
		"slices/mydir/mypkg1.yaml": `
			package: mypkg1
			slices:
				myslice1:
					contents:
						/path1:
		`,
		"slices/mydir/mypkg2.yaml": `
			package: mypkg2
			slices:
				myslice1:
					contents:
						/path1:
		`,
	},
	selslices: []setup.SliceKey{{"mypkg1", "myslice1"}, {"mypkg2", "myslice1"}},
	selerror:  "slices mypkg1.myslice1 and mypkg2.myslice1 conflict on /path1",
}, {
	summary: "Directories must be suffixed with /",
	input: map[string]string{
		"slices/mydir/mypkg.yaml": `
			package: mypkg
			slices:
				myslice:
					contents:
						/foo: {make: true}
		`,
	},
	relerror:  `slice mypkg.myslice content "/foo" must end in / for 'make' to be valid`,
}, {
	summary: "Slice path must be clean",
	input: map[string]string{
		"slices/mydir/mypkg.yaml": `
			package: mypkg
			slices:
				myslice:
					contents:
						/foo/../:
		`,
	},
	relerror:  `slice mypkg.myslice has invalid content path: /foo/../`,
}, {
	summary: "Slice path must be absolute",
	input: map[string]string{
		"slices/mydir/mypkg.yaml": `
			package: mypkg
			slices:
				myslice:
					contents:
						./foo/:
		`,
	},
	relerror:  `slice mypkg.myslice has invalid content path: ./foo/`,
}}

const defaultChiselYaml = `
	format: chisel-v1
	archives:
		ubuntu:
			version: 22.04
			components: [main, universe]
`

func (s *S) TestParseRelease(c *C) {
	for _, test := range setupTests {
		c.Logf("Summary: %s", test.summary)

		if _, ok := test.input["chisel.yaml"]; !ok {
			test.input["chisel.yaml"] = string(defaultChiselYaml)
		}

		dir := c.MkDir()
		for path, data := range test.input {
			fpath := filepath.Join(dir, path)
			err := os.MkdirAll(filepath.Dir(fpath), 0755)
			c.Assert(err, IsNil)
			err = ioutil.WriteFile(fpath, testutil.Reindent(data), 0644)
			c.Assert(err, IsNil)
		}

		release, err := setup.ReadRelease(dir)
		if err != nil || test.relerror != "" {
			if test.relerror != "" {
				c.Assert(err, ErrorMatches, test.relerror)
				continue
			} else {
				c.Assert(err, IsNil)
			}
		}

		c.Assert(release.Path, Equals, dir)
		release.Path = ""

		if test.release != nil {
			c.Assert(release, DeepEquals, test.release)
		}

		if test.selslices != nil {
			selection, err := setup.Select(release, test.selslices)
			if test.selerror != "" {
				c.Assert(err, ErrorMatches, test.selerror)
				continue
			} else {
				c.Assert(err, IsNil)
			}
			c.Assert(selection.Release, Equals, release)
			selection.Release = nil
			if test.selection != nil {
				c.Assert(selection, DeepEquals, test.selection)
			}
		}
	}
}
