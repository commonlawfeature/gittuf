// SPDX-License-Identifier: Apache-2.0

package policy

import (
	"github.com/gittuf/gittuf/internal/cmd/policy/addkey"
	"github.com/gittuf/gittuf/internal/cmd/policy/addrule"
	i "github.com/gittuf/gittuf/internal/cmd/policy/init"
	"github.com/gittuf/gittuf/internal/cmd/policy/persistent"
	"github.com/gittuf/gittuf/internal/cmd/policy/removerule"
	"github.com/gittuf/gittuf/internal/cmd/trustpolicy/remote"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	o := &persistent.Options{}
	cmd := &cobra.Command{
		Use:   "policy",
		Short: "Tools to manage gittuf policies",
	}
	o.AddPersistentFlags(cmd)

	cmd.AddCommand(i.New(o))
	cmd.AddCommand(addkey.New(o))
	cmd.AddCommand(addrule.New(o))
	cmd.AddCommand(remote.New())
	cmd.AddCommand(removerule.New(o))

	return cmd
}
