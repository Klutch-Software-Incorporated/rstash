package cli

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"gosilo/internal/auth"
	"gosilo/internal/db"

	"github.com/spf13/cobra"
)

var userCmd = &cobra.Command{
	Use:   "user",
	Short: "Manage users",
}

var userAddCmd = &cobra.Command{
	Use:   "add <username>",
	Short: "Create a new user",
	Args:  cobra.ExactArgs(1),
	RunE:  runUserAdd,
}

var userListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all users",
	RunE:  runUserList,
}

var userPasswdCmd = &cobra.Command{
	Use:   "passwd <username>",
	Short: "Change a user's password",
	Args:  cobra.ExactArgs(1),
	RunE:  runUserPasswd,
}

var userPromoteCmd = &cobra.Command{
	Use:   "promote <username>",
	Short: "Promote user to admin",
	Args:  cobra.ExactArgs(1),
	RunE:  runUserPromote,
}

var userDisableCmd = &cobra.Command{
	Use:   "disable <username>",
	Short: "Disable a user account",
	Args:  cobra.ExactArgs(1),
	RunE:  runUserDisable,
}

var userDeleteCmd = &cobra.Command{
	Use:   "delete <username>",
	Short: "Delete a user",
	Args:  cobra.ExactArgs(1),
	RunE:  runUserDelete,
}

var (
	userAddAdmin    bool
	userDisableFlag bool // --enable flag for re-enable
	userDeleteForce bool
)

func init() {
	userAddCmd.Flags().BoolVar(&userAddAdmin, "admin", false, "make the user an admin")
	userDisableCmd.Flags().BoolVar(&userDisableFlag, "enable", false, "re-enable the user instead of disabling")
	userDeleteCmd.Flags().BoolVar(&userDeleteForce, "force", false, "skip confirmation prompt")

	userCmd.AddCommand(userAddCmd, userListCmd, userPasswdCmd, userPromoteCmd, userDisableCmd, userDeleteCmd)
	rootCmd.AddCommand(userCmd)
}

func openAuthService() (*auth.LocalService, func(), error) {
	dsn := resolvedDBDSN("sqlite:gosilo.db")
	database, err := db.Open(dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("open database: %w", err)
	}
	svc := auth.NewLocalService(database)
	return svc, func() { database.Close() }, nil
}

func runUserAdd(cmd *cobra.Command, args []string) error {
	username := args[0]

	password, err := promptPassword("Password")
	if err != nil {
		return err
	}
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}

	confirm, err := promptPassword("Confirm password")
	if err != nil {
		return err
	}
	if password != confirm {
		return fmt.Errorf("passwords do not match")
	}

	svc, cleanup, err := openAuthService()
	if err != nil {
		return err
	}
	defer cleanup()

	user, err := svc.CreateUser(context.Background(), username, password, userAddAdmin)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}

	fmt.Fprintf(os.Stderr, "User %q created (ID: %d, admin: %v)\n", user.Username, user.ID, user.IsAdmin)
	return nil
}

func runUserList(cmd *cobra.Command, args []string) error {
	svc, cleanup, err := openAuthService()
	if err != nil {
		return err
	}
	defer cleanup()

	users, err := svc.ListUsers(context.Background())
	if err != nil {
		return fmt.Errorf("list users: %w", err)
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tUsername\tAdmin\tDisabled\tCreated")
	fmt.Fprintln(tw, "--\t--------\t-----\t--------\t-------")
	for _, u := range users {
		fmt.Fprintf(tw, "%d\t%s\t%v\t%v\t%s\n", u.ID, u.Username, u.IsAdmin, u.Disabled, u.CreatedAt)
	}
	return tw.Flush()
}

func runUserPasswd(cmd *cobra.Command, args []string) error {
	username := args[0]

	svc, cleanup, err := openAuthService()
	if err != nil {
		return err
	}
	defer cleanup()

	user, err := svc.GetUserByUsername(context.Background(), username)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}
	if user == nil {
		return fmt.Errorf("user %q not found", username)
	}

	password, err := promptPassword("New password")
	if err != nil {
		return err
	}
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}

	confirm, err := promptPassword("Confirm password")
	if err != nil {
		return err
	}
	if password != confirm {
		return fmt.Errorf("passwords do not match")
	}

	if err := svc.UpdatePassword(context.Background(), user.ID, password); err != nil {
		return fmt.Errorf("update password: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Password updated for %q.\n", username)
	return nil
}

func runUserPromote(cmd *cobra.Command, args []string) error {
	username := args[0]

	svc, cleanup, err := openAuthService()
	if err != nil {
		return err
	}
	defer cleanup()

	user, err := svc.GetUserByUsername(context.Background(), username)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}
	if user == nil {
		return fmt.Errorf("user %q not found", username)
	}

	if err := svc.ToggleAdmin(context.Background(), user.ID, true); err != nil {
		return fmt.Errorf("promote user: %w", err)
	}

	fmt.Fprintf(os.Stderr, "%q is now an admin.\n", username)
	return nil
}

func runUserDisable(cmd *cobra.Command, args []string) error {
	username := args[0]
	disable := !userDisableFlag // default: disable; --enable reverses

	svc, cleanup, err := openAuthService()
	if err != nil {
		return err
	}
	defer cleanup()

	user, err := svc.GetUserByUsername(context.Background(), username)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}
	if user == nil {
		return fmt.Errorf("user %q not found", username)
	}

	if err := svc.SetDisabled(context.Background(), user.ID, disable); err != nil {
		return fmt.Errorf("set disabled: %w", err)
	}

	if disable {
		// Terminate sessions when disabling.
		_ = svc.TerminateAllSessions(context.Background(), user.ID)
		fmt.Fprintf(os.Stderr, "%q has been disabled.\n", username)
	} else {
		fmt.Fprintf(os.Stderr, "%q has been enabled.\n", username)
	}
	return nil
}

func runUserDelete(cmd *cobra.Command, args []string) error {
	username := args[0]

	svc, cleanup, err := openAuthService()
	if err != nil {
		return err
	}
	defer cleanup()

	user, err := svc.GetUserByUsername(context.Background(), username)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}
	if user == nil {
		return fmt.Errorf("user %q not found", username)
	}

	if !userDeleteForce {
		answer, err := prompt(fmt.Sprintf("Delete user %q? (yes/no)", username))
		if err != nil {
			return err
		}
		if answer != "yes" {
			fmt.Fprintln(os.Stderr, "Aborted.")
			return nil
		}
	}

	if err := svc.DeleteUser(context.Background(), user.ID); err != nil {
		return fmt.Errorf("delete user: %w", err)
	}

	fmt.Fprintf(os.Stderr, "User %q deleted.\n", username)
	return nil
}
