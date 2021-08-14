package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
	"gopkg.in/yaml.v2"

	lxd "github.com/lxc/lxd/client"
	"github.com/lxc/lxd/lxc/utils"
	"github.com/lxc/lxd/lxd/cluster"
	"github.com/lxc/lxd/lxd/db"
	"github.com/lxc/lxd/lxd/sys"
	"github.com/lxc/lxd/lxd/util"
	"github.com/lxc/lxd/shared"
	"github.com/lxc/lxd/shared/termios"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

type cmdCluster struct {
	global *cmdGlobal
}

func (c *cmdCluster) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "cluster"
	cmd.Short = "Low-level cluster administration commands"
	cmd.Long = `Description:
  Low level administration tools for inspecting and recovering LXD clusters.
`
	// List database nodes
	listDatabase := cmdClusterListDatabase{global: c.global}
	cmd.AddCommand(listDatabase.Command())

	// Recover
	recover := cmdClusterRecoverFromQuorumLoss{global: c.global}
	cmd.AddCommand(recover.Command())

	// Remove a raft node.
	removeRaftNode := cmdClusterRemoveRaftNode{global: c.global}
	cmd.AddCommand(removeRaftNode.Command())

	// Edit cluster configuration.
	clusterEdit := cmdClusterEdit{global: c.global}
	cmd.AddCommand(clusterEdit.Command())

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { cmd.Usage() }
	return cmd
}

// ClusterMember is a more human-readable representation of the db.RaftNode struct.
type ClusterMember struct {
	ID      uint64 `yaml:"id"`
	Address string `yaml:"address"`
	Role    string `yaml:"role"`
}

// ClusterConfig is a representation of the current cluster configuration.
type ClusterConfig struct {
	Segment string          `yaml:"latest_segment"`
	Members []ClusterMember `yaml:"members"`
}

// ToRaftNode converts a ClusterConfig struct to a RaftNode struct.
func (c ClusterMember) ToRaftNode() (*db.RaftNode, error) {
	node := &db.RaftNode{
		ID:      c.ID,
		Address: c.Address,
	}

	var role db.RaftRole
	switch c.Role {
	case "voter":
		role = db.RaftVoter
		break
	case "stand-by":
		role = db.RaftStandBy
		break
	case "spare":
		role = db.RaftSpare
		break
	default:
		return nil, fmt.Errorf("unknown raft role: %q", c.Role)
	}

	node.Role = role

	return node, nil
}

type cmdClusterEdit struct {
	global *cmdGlobal
}

func (c *cmdClusterEdit) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "edit"
	cmd.Short = "Edit cluster configuration as YAML"
	cmd.Long = `Description:
	Edit cluster configuration as YAML.`
	cmd.RunE = c.Run

	return cmd
}

func (c *cmdClusterEdit) Run(cmd *cobra.Command, args []string) error {
	// Make sure that the daemon is not running.
	_, err := lxd.ConnectLXDUnix("", nil)
	if err == nil {
		return fmt.Errorf("The LXD daemon is running, please stop it first.")
	}

	database, _, err := db.OpenNode(filepath.Join(sys.DefaultOS().VarDir, "database"), nil, nil)
	if err != nil {
		return err
	}

	var nodes []db.RaftNode
	err = database.Transaction(func(tx *db.NodeTx) error {
		var err error
		nodes, err = tx.GetRaftNodes()
		return err
	})
	if err != nil {
		return err
	}

	segmentID, err := db.DqliteLatestSegment()
	if err != nil {
		return err
	}

	config := ClusterConfig{
		Segment: segmentID,
		Members: []ClusterMember{},
	}

	for _, node := range nodes {
		member := ClusterMember{ID: node.ID, Address: node.Address, Role: node.Role.String()}
		config.Members = append(config.Members, member)
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return err
	}

	var content []byte
	if !termios.IsTerminal(unix.Stdin) {
		content, err = ioutil.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
	} else {
		content, err = shared.TextEditor("", data)
		if err != nil {
			return err
		}
	}

	for {
		newConfig := ClusterConfig{}
		err = yaml.Unmarshal(content, &newConfig)
		if err == nil {
			// Convert ClusterConfig back to RaftNodes.
			newNodes := []db.RaftNode{}
			var newNode *db.RaftNode
			for _, node := range newConfig.Members {
				newNode, err = node.ToRaftNode()
				if err != nil {
					break
				}

				newNodes = append(newNodes, *newNode)
			}

			// Ensure new configuration is valid.
			if err == nil {
				err = cluster.Reconfigure(database, newNodes)
			}
		}

		if err != nil {
			fmt.Fprintf(os.Stderr, "Config validation error: %s\n", err)
			fmt.Println("Press enter to open the editor again or ctrl+c to abort change")
			_, err := os.Stdin.Read(make([]byte, 1))
			if err != nil {
				return err
			}

			content, err = shared.TextEditor("", content)
			if err != nil {
				return err
			}

			continue
		}

		break
	}

	return nil
}

type cmdClusterListDatabase struct {
	global *cmdGlobal
}

func (c *cmdClusterListDatabase) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "list-database"
	cmd.Aliases = []string{"ls"}
	cmd.Short = "Print the addresses of the cluster members serving the database"

	cmd.RunE = c.Run

	return cmd
}

func (c *cmdClusterListDatabase) Run(cmd *cobra.Command, args []string) error {
	os := sys.DefaultOS()

	db, _, err := db.OpenNode(filepath.Join(os.VarDir, "database"), nil, nil)
	if err != nil {
		return errors.Wrapf(err, "Failed to open local database")
	}

	addresses, err := cluster.ListDatabaseNodes(db)
	if err != nil {
		return errors.Wrapf(err, "Failed to get database nodes")
	}

	columns := []string{"Address"}
	data := make([][]string, len(addresses))
	for i, address := range addresses {
		data[i] = []string{address}
	}
	utils.RenderTable(utils.TableFormatTable, columns, data, nil)

	return nil
}

type cmdClusterRecoverFromQuorumLoss struct {
	global             *cmdGlobal
	flagNonInteractive bool
}

func (c *cmdClusterRecoverFromQuorumLoss) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "recover-from-quorum-loss"
	cmd.Short = "Recover a LXD instance whose cluster has lost quorum"

	cmd.RunE = c.Run

	cmd.Flags().BoolVarP(&c.flagNonInteractive, "quiet", "q", false, "Don't require user confirmation")

	return cmd
}

func (c *cmdClusterRecoverFromQuorumLoss) Run(cmd *cobra.Command, args []string) error {
	// Make sure that the daemon is not running.
	_, err := lxd.ConnectLXDUnix("", nil)
	if err == nil {
		return fmt.Errorf("The LXD daemon is running, please stop it first.")
	}

	// Prompt for confirmation unless --quiet was passed.
	if !c.flagNonInteractive {
		err := c.promptConfirmation()
		if err != nil {
			return err
		}
	}

	os := sys.DefaultOS()

	db, _, err := db.OpenNode(filepath.Join(os.VarDir, "database"), nil, nil)
	if err != nil {
		return errors.Wrapf(err, "Failed to open local database")
	}

	return cluster.Recover(db)
}

func (c *cmdClusterRecoverFromQuorumLoss) promptConfirmation() error {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf(`You should run this command only if you are *absolutely* certain that this is
the only database node left in your cluster AND that other database nodes will
never come back (i.e. their LXD daemon won't ever be started again).

This will make this LXD instance the only member of the cluster, and it won't
be possible to perform operations on former cluster members anymore.

However all information about former cluster members will be preserved in the
database, so you can possibly inspect it for further recovery.

You'll be able to permanently delete from the database all information about
former cluster members by running "lxc cluster remove <member-name> --force".

See https://linuxcontainers.org/lxd/docs/master/clustering#recover-from-quorum-loss for more
info.

Do you want to proceed? (yes/no): `)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSuffix(input, "\n")

	if !shared.StringInSlice(strings.ToLower(input), []string{"yes"}) {
		return fmt.Errorf("Recover operation aborted")
	}
	return nil
}

type cmdClusterRemoveRaftNode struct {
	global             *cmdGlobal
	flagNonInteractive bool
}

func (c *cmdClusterRemoveRaftNode) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "remove-raft-node <address>"
	cmd.Short = "Remove a raft node from the raft configuration"

	cmd.RunE = c.Run

	cmd.Flags().BoolVarP(&c.flagNonInteractive, "quiet", "q", false, "Don't require user confirmation")

	return cmd
}

func (c *cmdClusterRemoveRaftNode) Run(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		cmd.Help()
		return fmt.Errorf("Missing required arguments")
	}

	address := util.CanonicalNetworkAddress(args[0])

	// Prompt for confirmation unless --quiet was passed.
	if !c.flagNonInteractive {
		err := c.promptConfirmation()
		if err != nil {
			return err
		}
	}

	client, err := lxd.ConnectLXDUnix("", nil)
	if err != nil {
		return errors.Wrapf(err, "Failed to connect to LXD daemon")
	}

	endpoint := fmt.Sprintf("/internal/cluster/raft-node/%s", address)
	_, _, err = client.RawQuery("DELETE", endpoint, nil, "")
	if err != nil {
		return err
	}

	return nil
}

func (c *cmdClusterRemoveRaftNode) promptConfirmation() error {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf(`You should run this command only if you ended up in an
inconsistent state where a node has been uncleanly removed (i.e. it doesn't show
up in "lxc cluster list" but it's still in the raft configuration).

Do you want to proceed? (yes/no): `)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSuffix(input, "\n")

	if !shared.StringInSlice(strings.ToLower(input), []string{"yes"}) {
		return fmt.Errorf("Remove raft node operation aborted")
	}
	return nil
}
