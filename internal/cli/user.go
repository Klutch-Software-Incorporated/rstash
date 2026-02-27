package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"gosilo/internal/auth"
	"gosilo/internal/db"

	"github.com/spf13/cobra"
)

var userCmd = &cobra.Command{
	Use:     "user",
	Short:   "Manage users",
	Long:    "Create, list, modify, and delete user accounts.",
	GroupID: "users",
}

var userAddCmd = &cobra.Command{
	Use:   "add <username>",
	Short: "Create a new user",
	Long: `Create a new user account with the given username.

You will be prompted for a password interactively unless --password is provided.
Passwords must be at least 8 characters. Use --admin to grant the new user
administrator privileges immediately. The operation is recorded in the audit log.`,
	Example: `  gosilo user add alice
  gosilo user add alice --admin --password "s3cure-p4ss"`,
	Args: cobra.ExactArgs(1),
	RunE: runUserAdd,
}

var userListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all users",
	Long: `List all user accounts in a table showing ID, username, admin status, disabled
status, and creation date. Use --json to get the output as a JSON array for
scripting and integration with other tools.`,
	Example: `  gosilo user list
  gosilo user list --json`,
	RunE: runUserList,
}

var userPasswdCmd = &cobra.Command{
	Use:   "passwd <username>",
	Short: "Change a user's password",
	Long: `Change the password for an existing user. You will be prompted for the new
password interactively unless --password is provided. Passwords must be at
least 8 characters. Existing sessions are not terminated — use "user disable"
and "user disable --enable" to force re-authentication.`,
	Example: `  gosilo user passwd alice
  gosilo user passwd alice --password "new-p4ssword"`,
	Args: cobra.ExactArgs(1),
	RunE: runUserPasswd,
}

var userPromoteCmd = &cobra.Command{
	Use:   "promote <username>",
	Short: "Promote user to admin",
	Long: `Grant administrator privileges to a user. Admins can access the admin panel,
manage other users, change server settings, and view the audit log. This
operation is recorded in the audit log.`,
	Example: `  gosilo user promote alice`,
	Args:    cobra.ExactArgs(1),
	RunE:    runUserPromote,
}

var userDisableCmd = &cobra.Command{
	Use:   "disable <username>",
	Short: "Disable a user account",
	Long: `Disable a user account, preventing login and API access. All active sessions
are immediately terminated. The user's stored data is preserved. Use --enable
to re-enable a previously disabled account. The operation is recorded in the
audit log.`,
	Example: `  gosilo user disable alice
  gosilo user disable alice --enable`,
	Args: cobra.ExactArgs(1),
	RunE: runUserDisable,
}

var userDeleteCmd = &cobra.Command{
	Use:   "delete <username>",
	Short: "Delete a user",
	Long: `Permanently delete a user account. This removes the user record, all sessions,
and OAuth tokens. Stored files (blobs and node metadata) are NOT deleted and
must be cleaned up separately. A confirmation prompt is shown unless --force
is provided. The operation is recorded in the audit log.`,
	Example: `  gosilo user delete alice --force`,
	Args:    cobra.ExactArgs(1),
	RunE:    runUserDelete,
}

var userApproveCmd = &cobra.Command{
	Use:   "approve <username>",
	Short: "Approve a pending user",
	Long: `Approve a user account that is pending approval. This allows the user to log in.
Use --reject to delete the pending user instead of approving them.`,
	Example: `  gosilo user approve alice
  gosilo user approve alice --reject`,
	Args: cobra.ExactArgs(1),
	RunE: runUserApprove,
}

var (
	userAddAdmin     bool
	userPasswordFlag string // --password for non-interactive use
	userDisableFlag  bool   // --enable flag for re-enable
	userDeleteForce  bool
	userRejectFlag   bool   // --reject flag for user approve command
)

func init() {
	userAddCmd.Flags().BoolVar(&userAddAdmin, "admin", false, "make the user an admin")
	userAddCmd.Flags().StringVar(&userPasswordFlag, "password", "", "set password non-interactively")
	userPasswdCmd.Flags().StringVar(&userPasswordFlag, "password", "", "set password non-interactively")
	userDisableCmd.Flags().BoolVar(&userDisableFlag, "enable", false, "re-enable the user instead of disabling")
	userDeleteCmd.Flags().BoolVar(&userDeleteForce, "force", false, "skip confirmation prompt")
	userApproveCmd.Flags().BoolVar(&userRejectFlag, "reject", false, "reject (delete) the pending user instead of approving")

	userCmd.AddCommand(userAddCmd, userListCmd, userPasswdCmd, userPromoteCmd, userDisableCmd, userDeleteCmd, userApproveCmd)
	rootCmd.AddCommand(userCmd)
}

func openAuthService() (*auth.LocalService, *sql.DB, func(), error) {
	dsn := resolvedDBDSN("sqlite:gosilo.db")
	database, err := db.Open(dsn)
	if err != nil {
		return nil, nil, nil, &SystemError{fmt.Errorf("open database: %w", err)}
	}
	svc := auth.NewLocalService(database)
	return svc, database, func() { database.Close() }, nil
}

func runUserAdd(cmd *cobra.Command, args []string) error {
	username := args[0]

	password := userPasswordFlag
	if password == "" {
		var err error
		password, err = promptPassword("Password")
		if err != nil {
			return err
		}
		confirm, err := promptPassword("Confirm password")
		if err != nil {
			return err
		}
		if password != confirm {
			return fmt.Errorf("passwords do not match")
		}
	}
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}

	svc, database, cleanup, err := openAuthService()
	if err != nil {
		return err
	}
	defer cleanup()

	ctx := context.Background()
	user, err := svc.CreateUser(ctx, username, password, userAddAdmin, true)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}

	// Auto-accept TOS/Privacy for CLI-created users.
	_ = db.AcceptTOS(ctx, database, user.ID)
	_ = db.AcceptPrivacy(ctx, database, user.ID)

	db.Audit(ctx, database, db.SystemActorID, "user.created", "user", fmt.Sprintf("%d", user.ID), username)
	fmt.Fprintf(os.Stderr, "User %q created (ID: %d, admin: %v)\n", user.Username, user.ID, user.IsAdmin)
	return nil
}

func runUserList(cmd *cobra.Command, args []string) error {
	svc, _, cleanup, err := openAuthService()
	if err != nil {
		return err
	}
	defer cleanup()

	users, err := svc.ListUsers(context.Background())
	if err != nil {
		return fmt.Errorf("list users: %w", err)
	}

	if jsonFlag {
		type userJSON struct {
			ID                int64   `json:"id"`
			Username          string  `json:"username"`
			IsAdmin           bool    `json:"is_admin"`
			Disabled          bool    `json:"disabled"`
			Approved          bool    `json:"approved"`
			CreatedAt         string  `json:"created_at"`
			TOSAcceptedAt     *string `json:"tos_accepted_at,omitempty"`
			PrivacyAcceptedAt *string `json:"privacy_accepted_at,omitempty"`
		}
		out := make([]userJSON, len(users))
		for i, u := range users {
			out[i] = userJSON{
				ID:                u.ID,
				Username:          u.Username,
				IsAdmin:           u.IsAdmin,
				Disabled:          u.Disabled,
				Approved:          u.Approved,
				CreatedAt:         u.CreatedAt,
				TOSAcceptedAt:     u.TOSAcceptedAt,
				PrivacyAcceptedAt: u.PrivacyAcceptedAt,
			}
		}
		return json.NewEncoder(os.Stdout).Encode(out)
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tUsername\tAdmin\tDisabled\tApproved\tCreated")
	fmt.Fprintln(tw, "--\t--------\t-----\t--------\t--------\t-------")
	for _, u := range users {
		fmt.Fprintf(tw, "%d\t%s\t%v\t%v\t%v\t%s\n", u.ID, u.Username, u.IsAdmin, u.Disabled, u.Approved, u.CreatedAt)
	}
	return tw.Flush()
}

func runUserPasswd(cmd *cobra.Command, args []string) error {
	username := args[0]

	svc, database, cleanup, err := openAuthService()
	if err != nil {
		return err
	}
	defer cleanup()

	ctx := context.Background()
	user, err := svc.GetUserByUsername(ctx, username)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}
	if user == nil {
		return fmt.Errorf("user %q not found", username)
	}

	password := userPasswordFlag
	if password == "" {
		var err error
		password, err = promptPassword("New password")
		if err != nil {
			return err
		}
		confirm, err := promptPassword("Confirm password")
		if err != nil {
			return err
		}
		if password != confirm {
			return fmt.Errorf("passwords do not match")
		}
	}
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}

	if err := svc.UpdatePassword(ctx, user.ID, password); err != nil {
		return fmt.Errorf("update password: %w", err)
	}

	db.Audit(ctx, database, db.SystemActorID, "user.password_changed", "user", fmt.Sprintf("%d", user.ID), username)
	fmt.Fprintf(os.Stderr, "Password updated for %q.\n", username)
	return nil
}

func runUserPromote(cmd *cobra.Command, args []string) error {
	username := args[0]

	svc, database, cleanup, err := openAuthService()
	if err != nil {
		return err
	}
	defer cleanup()

	ctx := context.Background()
	user, err := svc.GetUserByUsername(ctx, username)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}
	if user == nil {
		return fmt.Errorf("user %q not found", username)
	}

	if err := svc.ToggleAdmin(ctx, user.ID, true); err != nil {
		return fmt.Errorf("promote user: %w", err)
	}

	db.Audit(ctx, database, db.SystemActorID, "user.promoted", "user", fmt.Sprintf("%d", user.ID), username)
	fmt.Fprintf(os.Stderr, "%q is now an admin.\n", username)
	return nil
}

func runUserDisable(cmd *cobra.Command, args []string) error {
	username := args[0]
	disable := !userDisableFlag // default: disable; --enable reverses

	svc, database, cleanup, err := openAuthService()
	if err != nil {
		return err
	}
	defer cleanup()

	ctx := context.Background()
	user, err := svc.GetUserByUsername(ctx, username)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}
	if user == nil {
		return fmt.Errorf("user %q not found", username)
	}

	if err := svc.SetDisabled(ctx, user.ID, disable); err != nil {
		return fmt.Errorf("set disabled: %w", err)
	}

	idStr := fmt.Sprintf("%d", user.ID)
	if disable {
		// Terminate sessions and revoke all tokens when disabling.
		_ = svc.TerminateAllSessions(ctx, user.ID)
		_ = db.DeleteOAuthTokensByUserID(ctx, database, user.ID)
		_ = db.DeleteRefreshTokensByUserID(ctx, database, user.ID)
		db.Audit(ctx, database, db.SystemActorID, "user.disabled", "user", idStr, username)
		fmt.Fprintf(os.Stderr, "%q has been disabled.\n", username)
	} else {
		db.Audit(ctx, database, db.SystemActorID, "user.enabled", "user", idStr, username)
		fmt.Fprintf(os.Stderr, "%q has been enabled.\n", username)
	}
	return nil
}

func runUserDelete(cmd *cobra.Command, args []string) error {
	username := args[0]

	svc, database, cleanup, err := openAuthService()
	if err != nil {
		return err
	}
	defer cleanup()

	ctx := context.Background()
	user, err := svc.GetUserByUsername(ctx, username)
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

	if err := svc.DeleteUser(ctx, user.ID); err != nil {
		return fmt.Errorf("delete user: %w", err)
	}

	db.Audit(ctx, database, db.SystemActorID, "user.deleted", "user", fmt.Sprintf("%d", user.ID), username)
	fmt.Fprintf(os.Stderr, "User %q deleted.\n", username)
	return nil
}

func runUserApprove(cmd *cobra.Command, args []string) error {
	username := args[0]

	svc, database, cleanup, err := openAuthService()
	if err != nil {
		return err
	}
	defer cleanup()

	ctx := context.Background()
	user, err := svc.GetUserByUsername(ctx, username)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}
	if user == nil {
		return fmt.Errorf("user %q not found", username)
	}

	idStr := fmt.Sprintf("%d", user.ID)

	if userRejectFlag {
		if err := svc.DeleteUser(ctx, user.ID); err != nil {
			return fmt.Errorf("delete user: %w", err)
		}
		db.Audit(ctx, database, db.SystemActorID, "user.rejected", "user", idStr, username)
		fmt.Fprintf(os.Stderr, "User %q rejected (deleted).\n", username)
		return nil
	}

	if err := svc.SetApproved(ctx, user.ID, true); err != nil {
		return fmt.Errorf("approve user: %w", err)
	}
	db.Audit(ctx, database, db.SystemActorID, "user.approved", "user", idStr, username)
	fmt.Fprintf(os.Stderr, "User %q approved.\n", username)
	return nil
}
