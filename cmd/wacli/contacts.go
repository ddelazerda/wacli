package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/steipete/wacli/internal/out"
)

func newContactsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "contacts",
		Short: "Search and manage local contact metadata",
	}
	cmd.AddCommand(newContactsSearchCmd(flags))
	cmd.AddCommand(newContactsShowCmd(flags))
	cmd.AddCommand(newContactsRefreshCmd(flags))
	cmd.AddCommand(newContactsAliasCmd(flags))
	cmd.AddCommand(newContactsTagsCmd(flags))
	return cmd
}

func newContactsSearchCmd(flags *rootFlags) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search contacts (from synced metadata)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := withTimeout(context.Background(), flags)
			defer cancel()

			a, lk, err := newApp(ctx, flags, false, false)
			if err != nil {
				return err
			}
			defer closeApp(a, lk)

			cs, err := a.DB().SearchContacts(args[0], limit)
			if err != nil {
				return err
			}

			if flags.asJSON {
				return out.WriteJSON(os.Stdout, cs)
			}

			w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
			fmt.Fprintln(w, "ALIAS\tNAME\tPHONE\tJID")
			for _, c := range cs {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
					truncate(c.Alias, 18),
					truncate(c.Name, 24),
					truncate(c.Phone, 14),
					c.JID,
				)
			}
			_ = w.Flush()
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 50, "limit results")
	return cmd
}

func newContactsShowCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show one contact",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := withTimeout(context.Background(), flags)
			defer cancel()

			a, lk, err := newApp(ctx, flags, false, false)
			if err != nil {
				return err
			}
			defer closeApp(a, lk)

			jid, err := requireJIDOrAlias(cmd, a.DB())
			if err != nil {
				return err
			}

			c, err := a.DB().GetContact(jid)
			if err != nil {
				return err
			}

			if flags.asJSON {
				return out.WriteJSON(os.Stdout, c)
			}

			fmt.Fprintf(os.Stdout, "JID: %s\n", c.JID)
			if c.Phone != "" {
				fmt.Fprintf(os.Stdout, "Phone: %s\n", c.Phone)
			}
			if c.Name != "" {
				fmt.Fprintf(os.Stdout, "Name: %s\n", c.Name)
			}
			if c.Alias != "" {
				fmt.Fprintf(os.Stdout, "Alias: %s\n", c.Alias)
			}
			if len(c.Tags) > 0 {
				fmt.Fprintf(os.Stdout, "Tags: %s\n", strings.Join(c.Tags, ", "))
			}
			return nil
		},
	}
	cmd.Flags().String("jid", "", "contact JID")
	cmd.Flags().String("alias", "", "contact alias (alternative to --jid)")
	return cmd
}

func newContactsRefreshCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "refresh",
		Short: "Import contacts from whatsmeow store into local DB",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := withTimeout(context.Background(), flags)
			defer cancel()

			a, lk, err := newApp(ctx, flags, true, true)
			if err != nil {
				return err
			}
			defer closeApp(a, lk)

			if err := a.OpenWA(); err != nil {
				return err
			}
			cs, err := a.WA().GetAllContacts(ctx)
			if err != nil {
				return err
			}

			var count int
			for jid, info := range cs {
				_ = a.DB().UpsertContact(
					jid.String(),
					jid.User,
					info.PushName,
					info.FullName,
					info.FirstName,
					info.BusinessName,
				)
				count++
			}

			if flags.asJSON {
				return out.WriteJSON(os.Stdout, map[string]any{"contacts": count})
			}
			fmt.Fprintf(os.Stdout, "Imported %d contacts.\n", count)
			return nil
		},
	}
	return cmd
}

// requireJIDOrAlias reads --jid and --alias from the command flags, enforces
// mutual exclusivity, and resolves --alias to a JID via the database.
// Returns the resolved JID.
func requireJIDOrAlias(cmd *cobra.Command, db interface {
	ResolveAlias(string) (string, error)
}) (string, error) {
	jid, _ := cmd.Flags().GetString("jid")
	alias, _ := cmd.Flags().GetString("alias")
	jid = strings.TrimSpace(jid)
	alias = strings.TrimSpace(alias)

	if jid != "" && alias != "" {
		return "", fmt.Errorf("--jid and --alias are mutually exclusive")
	}
	if jid == "" && alias == "" {
		return "", fmt.Errorf("--jid or --alias is required")
	}
	if jid != "" {
		return jid, nil
	}
	resolved, err := db.ResolveAlias(alias)
	if err != nil {
		return "", fmt.Errorf("failed to resolve alias %q: %w", alias, err)
	}
	if resolved == "" {
		return "", fmt.Errorf("alias %q not found", alias)
	}
	return resolved, nil
}

func newContactsAliasCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "alias",
		Short: "Manage local aliases",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all aliases",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := withTimeout(context.Background(), flags)
			defer cancel()
			a, lk, err := newApp(ctx, flags, false, false)
			if err != nil {
				return err
			}
			defer closeApp(a, lk)
			entries, err := a.DB().ListAliases()
			if err != nil {
				return err
			}
			if flags.asJSON {
				return out.WriteJSON(os.Stdout, entries)
			}
			if len(entries) == 0 {
				fmt.Fprintln(os.Stdout, "No aliases configured.")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
			fmt.Fprintln(w, "ALIAS\tJID")
			for _, e := range entries {
				fmt.Fprintf(w, "%s\t%s\n", e.Alias, e.JID)
			}
			_ = w.Flush()
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "set",
		Short: "Set alias",
		RunE: func(cmd *cobra.Command, args []string) error {
			jid, _ := cmd.Flags().GetString("jid")
			alias, _ := cmd.Flags().GetString("alias")
			if strings.TrimSpace(jid) == "" || strings.TrimSpace(alias) == "" {
				return fmt.Errorf("--jid and --alias are required")
			}
			ctx, cancel := withTimeout(context.Background(), flags)
			defer cancel()
			a, lk, err := newApp(ctx, flags, false, false)
			if err != nil {
				return err
			}
			defer closeApp(a, lk)
			if err := a.DB().SetAlias(jid, alias); err != nil {
				return err
			}
			if flags.asJSON {
				return out.WriteJSON(os.Stdout, map[string]any{"jid": jid, "alias": alias})
			}
			fmt.Fprintln(os.Stdout, "OK")
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "rm",
		Short: "Remove alias",
		RunE: func(cmd *cobra.Command, args []string) error {
			jid, _ := cmd.Flags().GetString("jid")
			alias, _ := cmd.Flags().GetString("alias")
			jid = strings.TrimSpace(jid)
			alias = strings.TrimSpace(alias)
			if jid != "" && alias != "" {
				return fmt.Errorf("--jid and --alias are mutually exclusive")
			}
			if jid == "" && alias == "" {
				return fmt.Errorf("--jid or --alias is required")
			}
			ctx, cancel := withTimeout(context.Background(), flags)
			defer cancel()
			a, lk, err := newApp(ctx, flags, false, false)
			if err != nil {
				return err
			}
			defer closeApp(a, lk)
			target := jid
			if target == "" {
				resolved, err := a.DB().ResolveAlias(alias)
				if err != nil {
					return fmt.Errorf("failed to resolve alias %q: %w", alias, err)
				}
				if resolved == "" {
					return fmt.Errorf("alias %q not found", alias)
				}
				target = resolved
			}
			if err := a.DB().RemoveAlias(target); err != nil {
				return err
			}
			if flags.asJSON {
				return out.WriteJSON(os.Stdout, map[string]any{"jid": target, "removed": true})
			}
			fmt.Fprintln(os.Stdout, "OK")
			return nil
		},
	})

	_ = cmd.PersistentFlags().String("jid", "", "contact JID")
	_ = cmd.PersistentFlags().String("alias", "", "alias")
	return cmd
}

func newContactsTagsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tags",
		Short: "Manage local tags",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "add",
		Short: "Add tag",
		RunE: func(cmd *cobra.Command, args []string) error {
			tag, _ := cmd.Flags().GetString("tag")
			if strings.TrimSpace(tag) == "" {
				return fmt.Errorf("--tag is required")
			}
			ctx, cancel := withTimeout(context.Background(), flags)
			defer cancel()
			a, lk, err := newApp(ctx, flags, false, false)
			if err != nil {
				return err
			}
			defer closeApp(a, lk)
			jid, err := requireJIDOrAlias(cmd, a.DB())
			if err != nil {
				return err
			}
			if err := a.DB().AddTag(jid, tag); err != nil {
				return err
			}
			if flags.asJSON {
				return out.WriteJSON(os.Stdout, map[string]any{"jid": jid, "tag": tag})
			}
			fmt.Fprintln(os.Stdout, "OK")
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "rm",
		Short: "Remove tag",
		RunE: func(cmd *cobra.Command, args []string) error {
			tag, _ := cmd.Flags().GetString("tag")
			if strings.TrimSpace(tag) == "" {
				return fmt.Errorf("--tag is required")
			}
			ctx, cancel := withTimeout(context.Background(), flags)
			defer cancel()
			a, lk, err := newApp(ctx, flags, false, false)
			if err != nil {
				return err
			}
			defer closeApp(a, lk)
			jid, err := requireJIDOrAlias(cmd, a.DB())
			if err != nil {
				return err
			}
			if err := a.DB().RemoveTag(jid, tag); err != nil {
				return err
			}
			if flags.asJSON {
				return out.WriteJSON(os.Stdout, map[string]any{"jid": jid, "tag": tag, "removed": true})
			}
			fmt.Fprintln(os.Stdout, "OK")
			return nil
		},
	})

	_ = cmd.PersistentFlags().String("jid", "", "contact JID")
	_ = cmd.PersistentFlags().String("alias", "", "contact alias (alternative to --jid)")
	_ = cmd.PersistentFlags().String("tag", "", "tag")
	return cmd
}
