package commands

import (
	"fmt"
	"io"
	"net/http"

	filestore "github.com/ipfs/go-ipfs/filestore"
	balanced "gx/ipfs/QmWv8MYwgPK4zXYv1et1snWJ6FWGqaL6xY2y9X1bRSKBxk/go-unixfs/importer/balanced"
	ihelper "gx/ipfs/QmWv8MYwgPK4zXYv1et1snWJ6FWGqaL6xY2y9X1bRSKBxk/go-unixfs/importer/helpers"
	trickle "gx/ipfs/QmWv8MYwgPK4zXYv1et1snWJ6FWGqaL6xY2y9X1bRSKBxk/go-unixfs/importer/trickle"

	cmdkit "gx/ipfs/QmPVqQHEfLpqK7JLCsUkyam7rhuV3MAeZ9gueQQCrBwCta/go-ipfs-cmdkit"
	mh "gx/ipfs/QmPnFwZ2JXKnXgMw8CdBPxn7FWh6LLdjUjxV1fKHuJnkr8/go-multihash"
	chunk "gx/ipfs/QmWbCAB5f3LDumj4ncz1UCHSiyXrXxkMxZB6Wv35xi4P8z/go-ipfs-chunker"
	cid "gx/ipfs/QmYjnkEL7i731PirfVH1sis89evN7jt4otSHw5D2xXXwUV/go-cid"
	cmds "gx/ipfs/QmdwAKtb9JX4VwnKRhjkxEcU2aMuXZZVLBNWtLZGE4UcPW/go-ipfs-cmds"
)

var urlStoreCmd = &cmds.Command{
	Subcommands: map[string]*cmds.Command{
		"add": urlAdd,
	},
}

var urlAdd = &cmds.Command{
	Helptext: cmdkit.HelpText{
		Tagline: "Add URL via urlstore.",
		LongDescription: `
Add URLs to ipfs without storing the data locally.

The URL provided must be stable and ideally on a web server under your
control.

The file is added using raw-leaves but otherwise using the default
settings for 'ipfs add'.

The file is not pinned, so this command should be followed by an 'ipfs
pin add'.

This command is considered temporary until a better solution can be
found.  It may disappear or the semantics can change at any
time.
`,
	},
	Options: []cmdkit.Option{
		cmdkit.BoolOption(trickleOptionName, "t", "Use trickle-dag format for dag generation."),
	},
	Arguments: []cmdkit.Argument{
		cmdkit.StringArg("url", true, false, "URL to add to IPFS"),
	},
	Type: &BlockStat{},

	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		url := req.Arguments[0]
		n, err := GetNode(env)
		if err != nil {
			return err
		}

		if !filestore.IsURL(url) {
			return fmt.Errorf("unsupported url syntax: %s", url)
		}

		cfg, err := n.Repo.Config()
		if err != nil {
			return err
		}

		if !cfg.Experimental.UrlstoreEnabled {
			return filestore.ErrUrlstoreNotEnabled
		}

		useTrickledag, _ := req.Options[trickleOptionName].(bool)

		hreq, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return err
		}

		hres, err := http.DefaultClient.Do(hreq)
		if err != nil {
			return err
		}
		if hres.StatusCode != http.StatusOK {
			return fmt.Errorf("expected code 200, got: %d", hres.StatusCode)
		}

		chk := chunk.NewSizeSplitter(hres.Body, chunk.DefaultBlockSize)
		prefix := cid.NewPrefixV1(cid.DagProtobuf, mh.SHA2_256)
		dbp := &ihelper.DagBuilderParams{
			Dagserv:    n.DAG,
			RawLeaves:  true,
			Maxlinks:   ihelper.DefaultLinksPerBlock,
			NoCopy:     true,
			CidBuilder: &prefix,
			URL:        url,
		}

		layout := balanced.Layout
		if useTrickledag {
			layout = trickle.Layout
		}
		root, err := layout(dbp.New(chk))
		if err != nil {
			return err
		}

		err = cmds.EmitOnce(res, &BlockStat{
			Key:  root.Cid().String(),
			Size: int(hres.ContentLength),
		})
		return err
	},
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeTypedEncoder(func(req *cmds.Request, w io.Writer, bs *BlockStat) error {
			_, err := fmt.Fprintln(w, bs.Key)
			return err
		}),
	},
}
