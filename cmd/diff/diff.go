package diff

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/itchio/butler/comm"
	"github.com/itchio/butler/filtering"
	"github.com/itchio/butler/mansion"
	"github.com/itchio/httpkit/progress"
	"github.com/itchio/savior/seeksource"
	"github.com/itchio/wharf/counter"
	"github.com/itchio/wharf/eos"
	"github.com/itchio/wharf/eos/option"
	"github.com/itchio/wharf/pools"
	"github.com/itchio/wharf/pools/nullpool"
	"github.com/itchio/wharf/pwr"
	"github.com/itchio/wharf/tlc"
	"github.com/itchio/wharf/wire"
	"github.com/itchio/wharf/wsync"
	"github.com/pkg/errors"
)

var args = struct {
	old    *string
	new    *string
	patch  *string
	verify *bool
}{}

func Register(ctx *mansion.Context) {
	cmd := ctx.App.Command("diff", "(Advanced) Compute the difference between two directories or .zip archives. Stores the patch in `patch.pwr`, and a signature in `patch.pwr.sig` for integrity checks and further diff.")
	args.old = cmd.Arg("old", "Directory or .zip archive (slower) with older files, or signature file generated from old directory.").Required().String()
	args.new = cmd.Arg("new", "Directory or .zip archive (slower) with newer files").Required().String()
	args.patch = cmd.Arg("patch", "Path to write the patch file (recommended extension is `.pwr`) The signature file will be written to the same path, with .sig added to the end.").Default("patch.pwr").String()
	args.verify = cmd.Flag("verify", "Make sure generated patch applies cleanly by applying it (slower)").Bool()
	ctx.Register(cmd, do)
}

type Params struct {
	// Target is the old version of the data
	Target string
	// Source is the new version of the data
	Source string
	// Patch is where to write the patch
	Patch       string
	Compression pwr.CompressionSettings
	// Verify enables dry-run apply patch validation (slow)
	Verify bool
}

func do(ctx *mansion.Context) {
	ctx.Must(Do(&Params{
		Target:      *args.old,
		Source:      *args.new,
		Patch:       *args.patch,
		Compression: ctx.CompressionSettings(),
		Verify:      *args.verify,
	}))
}

func Do(params *Params) error {
	var err error

	startTime := time.Now()

	targetSignature := &pwr.SignatureInfo{}

	if params.Target == "" {
		return errors.New("diff: must specify Target")
	}
	if params.Source == "" {
		return errors.New("diff: must specify Source")
	}
	if params.Patch == "" {
		return errors.New("diff: must specify Patch")
	}

	readAsSignature := func() error {
		// Signature file perhaps?
		signatureReader, err := eos.Open(params.Target, option.WithConsumer(comm.NewStateConsumer()))
		if err != nil {
			return errors.Wrap(err, "opening target")
		}
		defer signatureReader.Close()

		stats, _ := signatureReader.Stat()
		if stats.IsDir() {
			return wire.ErrFormat
		}

		signatureSource := seeksource.FromFile(signatureReader)
		_, err = signatureSource.Resume(nil)
		if err != nil {
			return errors.Wrap(err, "opening target")
		}

		readSignature, err := pwr.ReadSignature(context.Background(), signatureSource)
		if err != nil {
			return errors.Wrap(err, "reading target as signature")
		}

		targetSignature = readSignature

		comm.Opf("Read signature from %s", params.Target)

		return nil
	}

	err = readAsSignature()

	if err != nil {
		if errors.Cause(err) == wire.ErrFormat || errors.Cause(err) == io.EOF {
			// must be a container then
			targetSignature.Container, err = tlc.WalkAny(params.Target, &tlc.WalkOpts{Filter: filtering.FilterPaths})
			// Container (dir, archive, etc.)
			comm.Opf("Hashing %s", params.Target)

			comm.StartProgress()
			var targetPool wsync.Pool
			targetPool, err = pools.New(targetSignature.Container, params.Target)
			if err != nil {
				return errors.Wrap(err, "opening target as directory")
			}

			targetSignature.Hashes, err = pwr.ComputeSignature(context.Background(), targetSignature.Container, targetPool, comm.NewStateConsumer())
			comm.EndProgress()
			if err != nil {
				return errors.Wrap(err, "computing target signature")
			}

			{
				prettySize := progress.FormatBytes(targetSignature.Container.Size)
				perSecond := progress.FormatBPS(targetSignature.Container.Size, time.Since(startTime))
				comm.Statf("%s (%s) @ %s/s\n", prettySize, targetSignature.Container.Stats(), perSecond)
			}
		} else {
			return errors.Wrap(err, "determining if target is signature or directory")
		}
	}

	startTime = time.Now()

	var sourceContainer *tlc.Container
	sourceContainer, err = tlc.WalkAny(params.Source, &tlc.WalkOpts{Filter: filtering.FilterPaths})
	if err != nil {
		return errors.Wrap(err, "walking source as directory")
	}

	var sourcePool wsync.Pool
	sourcePool, err = pools.New(sourceContainer, params.Source)
	if err != nil {
		return errors.Wrap(err, "walking source as directory")
	}

	patchWriter, err := os.Create(params.Patch)
	if err != nil {
		return errors.Wrap(err, "creating patch file")
	}
	defer patchWriter.Close()

	signaturePath := params.Patch + ".sig"
	signatureWriter, err := os.Create(signaturePath)
	if err != nil {
		return errors.Wrap(err, "creating signature file")
	}
	defer signatureWriter.Close()

	patchCounter := counter.NewWriter(patchWriter)
	signatureCounter := counter.NewWriter(signatureWriter)

	dctx := &pwr.DiffContext{
		SourceContainer: sourceContainer,
		Pool:            sourcePool,

		TargetContainer: targetSignature.Container,
		TargetSignature: targetSignature.Hashes,

		Consumer:    comm.NewStateConsumer(),
		Compression: &params.Compression,
	}

	comm.Opf("Diffing %s", params.Source)
	comm.StartProgress()
	err = dctx.WritePatch(context.Background(), patchCounter, signatureCounter)
	if err != nil {
		return errors.Wrap(err, "computing and writing patch and signature")
	}
	comm.EndProgress()

	totalDuration := time.Since(startTime)
	{
		prettySize := progress.FormatBytes(sourceContainer.Size)
		perSecond := progress.FormatBPS(sourceContainer.Size, totalDuration)
		comm.Statf("%s (%s) @ %s/s\n", prettySize, sourceContainer.Stats(), perSecond)
	}

	{
		prettyPatchSize := progress.FormatBytes(patchCounter.Count())
		percReused := 100.0 * float64(dctx.ReusedBytes) / float64(dctx.FreshBytes+dctx.ReusedBytes)
		relToNew := 100.0 * float64(patchCounter.Count()) / float64(sourceContainer.Size)
		prettyFreshSize := progress.FormatBytes(dctx.FreshBytes)

		comm.Statf("Re-used %.2f%% of old, added %s fresh data", percReused, prettyFreshSize)
		comm.Statf("%s patch (%.2f%% of the full size) in %s", prettyPatchSize, relToNew, totalDuration)
	}

	if params.Verify {
		comm.Opf("Applying patch to verify it...")
		_, err := signatureWriter.Seek(0, io.SeekStart)
		if err != nil {
			return errors.Wrap(err, "seeking to beginning of fresh signature file")
		}

		signatureSource := seeksource.FromFile(signatureWriter)

		_, err = signatureSource.Resume(nil)
		if err != nil {
			return errors.Wrap(err, "reading fresh signature file")
		}

		signature, err := pwr.ReadSignature(context.Background(), signatureSource)
		if err != nil {
			return errors.Wrap(err, "decoding fresh signature file")
		}

		actx := &pwr.ApplyContext{
			OutputPool: &pwr.ValidatingPool{
				Pool:      nullpool.New(sourceContainer),
				Container: sourceContainer,
				Signature: signature,
			},
			TargetPath:      params.Target,
			TargetContainer: targetSignature.Container,

			SourceContainer: sourceContainer,

			Consumer: comm.NewStateConsumer(),
		}

		patchSource := seeksource.FromFile(patchWriter)

		_, err = patchSource.Resume(nil)
		if err != nil {
			return errors.Wrap(err, "creating source for patch")
		}

		comm.StartProgress()
		err = actx.ApplyPatch(patchSource)
		comm.EndProgress()
		if err != nil {
			return errors.Wrap(err, "applying patch")
		}

		comm.Statf("Patch applies cleanly!")
	}

	return nil
}
