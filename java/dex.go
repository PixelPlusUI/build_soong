// Copyright 2017 Google Inc. All rights reserved.
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

package java

import (
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/remoteexec"
)

type DexProperties struct {
	// If set to true, compile dex regardless of installable.  Defaults to false.
	Compile_dex *bool

	// list of module-specific flags that will be used for dex compiles
	Dxflags []string `android:"arch_variant"`

	Optimize struct {
		// If false, disable all optimization.  Defaults to true for android_app and android_test
		// modules, false for java_library and java_test modules.
		Enabled *bool
		// True if the module containing this has it set by default.
		EnabledByDefault bool `blueprint:"mutated"`

		// If true, runs R8 in Proguard compatibility mode (default).
		// Otherwise, runs R8 in full mode.
		Proguard_compatibility *bool

		// If true, optimize for size by removing unused code.  Defaults to true for apps,
		// false for libraries and tests.
		Shrink *bool

		// If true, optimize bytecode.  Defaults to false.
		Optimize *bool

		// If true, obfuscate bytecode.  Defaults to false.
		Obfuscate *bool

		// If true, do not use the flag files generated by aapt that automatically keep
		// classes referenced by the app manifest.  Defaults to false.
		No_aapt_flags *bool

		// Flags to pass to proguard.
		Proguard_flags []string

		// Specifies the locations of files containing proguard flags.
		Proguard_flags_files []string `android:"path"`
	}

	// Keep the data uncompressed. We always need uncompressed dex for execution,
	// so this might actually save space by avoiding storing the same data twice.
	// This defaults to reasonable value based on module and should not be set.
	// It exists only to support ART tests.
	Uncompress_dex *bool
}

type dexer struct {
	dexProperties DexProperties

	// list of extra proguard flag files
	extraProguardFlagFiles android.Paths
	proguardDictionary     android.OptionalPath
	proguardUsageZip       android.OptionalPath
}

func (d *dexer) effectiveOptimizeEnabled() bool {
	return BoolDefault(d.dexProperties.Optimize.Enabled, d.dexProperties.Optimize.EnabledByDefault)
}

var d8, d8RE = remoteexec.MultiCommandStaticRules(pctx, "d8",
	blueprint.RuleParams{
		Command: `rm -rf "$outDir" && mkdir -p "$outDir" && ` +
			`$d8Template${config.D8Cmd} ${config.DexFlags} --output $outDir $d8Flags $in && ` +
			`$zipTemplate${config.SoongZipCmd} $zipFlags -o $outDir/classes.dex.jar -C $outDir -f "$outDir/classes*.dex" && ` +
			`${config.MergeZipsCmd} -D -stripFile "**/*.class" $out $outDir/classes.dex.jar $in`,
		CommandDeps: []string{
			"${config.D8Cmd}",
			"${config.SoongZipCmd}",
			"${config.MergeZipsCmd}",
		},
	}, map[string]*remoteexec.REParams{
		"$d8Template": &remoteexec.REParams{
			Labels:          map[string]string{"type": "compile", "compiler": "d8"},
			Inputs:          []string{"${config.D8Jar}"},
			ExecStrategy:    "${config.RED8ExecStrategy}",
			ToolchainInputs: []string{"${config.JavaCmd}"},
			Platform:        map[string]string{remoteexec.PoolKey: "${config.REJavaPool}"},
		},
		"$zipTemplate": &remoteexec.REParams{
			Labels:       map[string]string{"type": "tool", "name": "soong_zip"},
			Inputs:       []string{"${config.SoongZipCmd}", "$outDir"},
			OutputFiles:  []string{"$outDir/classes.dex.jar"},
			ExecStrategy: "${config.RED8ExecStrategy}",
			Platform:     map[string]string{remoteexec.PoolKey: "${config.REJavaPool}"},
		},
	}, []string{"outDir", "d8Flags", "zipFlags"}, nil)

var r8, r8RE = remoteexec.MultiCommandStaticRules(pctx, "r8",
	blueprint.RuleParams{
		Command: `rm -rf "$outDir" && mkdir -p "$outDir" && ` +
			`rm -f "$outDict" && rm -rf "${outUsageDir}" && ` +
			`mkdir -p $$(dirname ${outUsage}) && ` +
			`$r8Template${config.R8Cmd} ${config.DexFlags} -injars $in --output $outDir ` +
			`--no-data-resources ` +
			`-printmapping ${outDict} ` +
			`-printusage ${outUsage} ` +
			`$r8Flags && ` +
			`touch "${outDict}" "${outUsage}" && ` +
			`${config.SoongZipCmd} -o ${outUsageZip} -C ${outUsageDir} -f ${outUsage} && ` +
			`rm -rf ${outUsageDir} && ` +
			`$zipTemplate${config.SoongZipCmd} $zipFlags -o $outDir/classes.dex.jar -C $outDir -f "$outDir/classes*.dex" && ` +
			`${config.MergeZipsCmd} -D -stripFile "**/*.class" $out $outDir/classes.dex.jar $in`,
		CommandDeps: []string{
			"${config.R8Cmd}",
			"${config.SoongZipCmd}",
			"${config.MergeZipsCmd}",
		},
	}, map[string]*remoteexec.REParams{
		"$r8Template": &remoteexec.REParams{
			Labels:          map[string]string{"type": "compile", "compiler": "r8"},
			Inputs:          []string{"$implicits", "${config.R8Jar}"},
			ExecStrategy:    "${config.RER8ExecStrategy}",
			ToolchainInputs: []string{"${config.JavaCmd}"},
			Platform:        map[string]string{remoteexec.PoolKey: "${config.REJavaPool}"},
		},
		"$zipTemplate": &remoteexec.REParams{
			Labels:       map[string]string{"type": "tool", "name": "soong_zip"},
			Inputs:       []string{"${config.SoongZipCmd}", "$outDir"},
			OutputFiles:  []string{"$outDir/classes.dex.jar"},
			ExecStrategy: "${config.RER8ExecStrategy}",
			Platform:     map[string]string{remoteexec.PoolKey: "${config.REJavaPool}"},
		},
		"$zipUsageTemplate": &remoteexec.REParams{
			Labels:       map[string]string{"type": "tool", "name": "soong_zip"},
			Inputs:       []string{"${config.SoongZipCmd}", "${outUsage}"},
			OutputFiles:  []string{"${outUsageZip}"},
			ExecStrategy: "${config.RER8ExecStrategy}",
			Platform:     map[string]string{remoteexec.PoolKey: "${config.REJavaPool}"},
		},
	}, []string{"outDir", "outDict", "outUsage", "outUsageZip", "outUsageDir",
		"r8Flags", "zipFlags"}, []string{"implicits"})

func (d *dexer) dexCommonFlags(ctx android.ModuleContext, minSdkVersion sdkSpec) []string {
	flags := d.dexProperties.Dxflags
	// Translate all the DX flags to D8 ones until all the build files have been migrated
	// to D8 flags. See: b/69377755
	flags = android.RemoveListFromList(flags,
		[]string{"--core-library", "--dex", "--multi-dex"})

	if ctx.Config().Getenv("NO_OPTIMIZE_DX") != "" {
		flags = append(flags, "--debug")
	}

	if ctx.Config().Getenv("GENERATE_DEX_DEBUG") != "" {
		flags = append(flags,
			"--debug",
			"--verbose")
	}

	effectiveVersion, err := minSdkVersion.effectiveVersion(ctx)
	if err != nil {
		ctx.PropertyErrorf("min_sdk_version", "%s", err)
	}

	flags = append(flags, "--min-api "+effectiveVersion.asNumberString())
	return flags
}

func d8Flags(flags javaBuilderFlags) (d8Flags []string, d8Deps android.Paths) {
	d8Flags = append(d8Flags, flags.bootClasspath.FormRepeatedClassPath("--lib ")...)
	d8Flags = append(d8Flags, flags.classpath.FormRepeatedClassPath("--lib ")...)

	d8Deps = append(d8Deps, flags.bootClasspath...)
	d8Deps = append(d8Deps, flags.classpath...)

	return d8Flags, d8Deps
}

func (d *dexer) r8Flags(ctx android.ModuleContext, flags javaBuilderFlags) (r8Flags []string, r8Deps android.Paths) {
	opt := d.dexProperties.Optimize

	// When an app contains references to APIs that are not in the SDK specified by
	// its LOCAL_SDK_VERSION for example added by support library or by runtime
	// classes added by desugaring, we artifically raise the "SDK version" "linked" by
	// ProGuard, to
	// - suppress ProGuard warnings of referencing symbols unknown to the lower SDK version.
	// - prevent ProGuard stripping subclass in the support library that extends class added in the higher SDK version.
	// See b/20667396
	var proguardRaiseDeps classpath
	ctx.VisitDirectDepsWithTag(proguardRaiseTag, func(dep android.Module) {
		proguardRaiseDeps = append(proguardRaiseDeps, dep.(Dependency).HeaderJars()...)
	})

	r8Flags = append(r8Flags, proguardRaiseDeps.FormJavaClassPath("-libraryjars"))
	r8Flags = append(r8Flags, flags.bootClasspath.FormJavaClassPath("-libraryjars"))
	r8Flags = append(r8Flags, flags.classpath.FormJavaClassPath("-libraryjars"))

	r8Deps = append(r8Deps, proguardRaiseDeps...)
	r8Deps = append(r8Deps, flags.bootClasspath...)
	r8Deps = append(r8Deps, flags.classpath...)

	flagFiles := android.Paths{
		android.PathForSource(ctx, "build/make/core/proguard.flags"),
	}

	flagFiles = append(flagFiles, d.extraProguardFlagFiles...)
	// TODO(ccross): static android library proguard files

	flagFiles = append(flagFiles, android.PathsForModuleSrc(ctx, opt.Proguard_flags_files)...)

	r8Flags = append(r8Flags, android.JoinWithPrefix(flagFiles.Strings(), "-include "))
	r8Deps = append(r8Deps, flagFiles...)

	// TODO(b/70942988): This is included from build/make/core/proguard.flags
	r8Deps = append(r8Deps, android.PathForSource(ctx,
		"build/make/core/proguard_basic_keeps.flags"))

	r8Flags = append(r8Flags, opt.Proguard_flags...)

	if BoolDefault(opt.Proguard_compatibility, true) {
		r8Flags = append(r8Flags, "--force-proguard-compatibility")
	}

	// TODO(ccross): Don't shrink app instrumentation tests by default.
	if !Bool(opt.Shrink) {
		r8Flags = append(r8Flags, "-dontshrink")
	}

	if !Bool(opt.Optimize) {
		r8Flags = append(r8Flags, "-dontoptimize")
	}

	// TODO(ccross): error if obufscation + app instrumentation test.
	if !Bool(opt.Obfuscate) {
		r8Flags = append(r8Flags, "-dontobfuscate")
	}
	// TODO(ccross): if this is an instrumentation test of an obfuscated app, use the
	// dictionary of the app and move the app from libraryjars to injars.

	// Don't strip out debug information for eng builds.
	if ctx.Config().Eng() {
		r8Flags = append(r8Flags, "--debug")
	}

	return r8Flags, r8Deps
}

func (d *dexer) compileDex(ctx android.ModuleContext, flags javaBuilderFlags, minSdkVersion sdkSpec,
	classesJar android.Path, jarName string) android.ModuleOutPath {

	// Compile classes.jar into classes.dex and then javalib.jar
	javalibJar := android.PathForModuleOut(ctx, "dex", jarName)
	outDir := android.PathForModuleOut(ctx, "dex")

	zipFlags := "--ignore_missing_files"
	if proptools.Bool(d.dexProperties.Uncompress_dex) {
		zipFlags += " -L 0"
	}

	commonFlags := d.dexCommonFlags(ctx, minSdkVersion)

	useR8 := d.effectiveOptimizeEnabled()
	if useR8 {
		proguardDictionary := android.PathForModuleOut(ctx, "proguard_dictionary")
		d.proguardDictionary = android.OptionalPathForPath(proguardDictionary)
		proguardUsageDir := android.PathForModuleOut(ctx, "proguard_usage")
		proguardUsage := proguardUsageDir.Join(ctx, ctx.Namespace().Path,
			android.ModuleNameWithPossibleOverride(ctx), "unused.txt")
		proguardUsageZip := android.PathForModuleOut(ctx, "proguard_usage.zip")
		d.proguardUsageZip = android.OptionalPathForPath(proguardUsageZip)
		r8Flags, r8Deps := d.r8Flags(ctx, flags)
		rule := r8
		args := map[string]string{
			"r8Flags":     strings.Join(append(commonFlags, r8Flags...), " "),
			"zipFlags":    zipFlags,
			"outDict":     proguardDictionary.String(),
			"outUsageDir": proguardUsageDir.String(),
			"outUsage":    proguardUsage.String(),
			"outUsageZip": proguardUsageZip.String(),
			"outDir":      outDir.String(),
		}
		if ctx.Config().UseRBE() && ctx.Config().IsEnvTrue("RBE_R8") {
			rule = r8RE
			args["implicits"] = strings.Join(r8Deps.Strings(), ",")
		}
		ctx.Build(pctx, android.BuildParams{
			Rule:            rule,
			Description:     "r8",
			Output:          javalibJar,
			ImplicitOutputs: android.WritablePaths{proguardDictionary, proguardUsageZip},
			Input:           classesJar,
			Implicits:       r8Deps,
			Args:            args,
		})
	} else {
		d8Flags, d8Deps := d8Flags(flags)
		rule := d8
		if ctx.Config().UseRBE() && ctx.Config().IsEnvTrue("RBE_D8") {
			rule = d8RE
		}
		ctx.Build(pctx, android.BuildParams{
			Rule:        rule,
			Description: "d8",
			Output:      javalibJar,
			Input:       classesJar,
			Implicits:   d8Deps,
			Args: map[string]string{
				"d8Flags":  strings.Join(append(commonFlags, d8Flags...), " "),
				"zipFlags": zipFlags,
				"outDir":   outDir.String(),
			},
		})
	}
	if proptools.Bool(d.dexProperties.Uncompress_dex) {
		alignedJavalibJar := android.PathForModuleOut(ctx, "aligned", jarName)
		TransformZipAlign(ctx, alignedJavalibJar, javalibJar)
		javalibJar = alignedJavalibJar
	}

	return javalibJar
}
