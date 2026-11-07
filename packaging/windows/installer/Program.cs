using System.Diagnostics;
using System.Drawing;
using System.Runtime.InteropServices;
using System.Runtime.Versioning;
using System.Windows.Forms;

internal static class Program
{
    private const string ProductName = "Portly";

    [STAThread]
    [SupportedOSPlatform("windows")]
    private static int Main(string[] args)
    {
        var sourceRoot = AppContext.BaseDirectory;
        try
        {
            var options = SetupOptions.Parse(args);
            if (options.ShowHelp)
            {
                Console.WriteLine("PortlySetup.exe [--install-root <path>] [--skip-path] [--skip-skill] [--skip-rules] [--skip-shortcut]");
                return 0;
            }

            if (args.Length == 0)
            {
                ApplicationConfiguration.Initialize();
                using var form = new InstallerForm(sourceRoot);
                Application.Run(form);
                return form.ExitCode;
            }

            Install(sourceRoot, options, Console.WriteLine);
            return 0;
        }
        catch (Exception error)
        {
            if (args.Length == 0)
            {
                MessageBox.Show(
                    $"Portly n'a pas pu être installé.\n\n{error.Message}",
                    "Installation Portly",
                    MessageBoxButtons.OK,
                    MessageBoxIcon.Error);
            }
            else
            {
                Console.Error.WriteLine($"Portly installation failed: {error.Message}");
            }
            return 1;
        }
    }

    private static void Install(string sourceRoot, SetupOptions options, Action<string> report)
    {
        var installRoot = options.InstallRoot ?? Path.Combine(
            Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData), ProductName);
        var binRoot = Path.Combine(installRoot, "bin");

        report($"Installation dans {installRoot}");
        CopyRequired(Path.Combine(sourceRoot, "Portly.exe"), Path.Combine(installRoot, "Portly.exe"));
        CopyRequired(Path.Combine(sourceRoot, "cli", "portly.exe"), Path.Combine(binRoot, "portly.exe"));
        Directory.CreateDirectory(Path.Combine(installRoot, "data"));
        Directory.CreateDirectory(Path.Combine(
            Environment.GetFolderPath(Environment.SpecialFolder.UserProfile),
            ".config", "portly", "logs"));

        var uninstallSource = Path.Combine(sourceRoot, "packaging", "windows", "Uninstall-Portly.ps1");
        if (File.Exists(uninstallSource))
        {
            CopyRequired(uninstallSource, Path.Combine(installRoot, "Uninstall-Portly.ps1"));
        }

        if (!options.SkipPath)
        {
            AddToUserPath(binRoot);
            report("CLI ajoutée au PATH utilisateur");
        }
        if (!options.SkipSkill)
        {
            InstallSkillIfMissing(Path.Combine(sourceRoot, "skills", "portly"), report);
        }
        if (!options.SkipRules)
        {
            InstallGlobalRules();
            report("Règles Portly installées");
        }
        if (!options.SkipShortcut)
        {
            CreateStartMenuShortcut(installRoot);
            report("Raccourci du menu Démarrer créé");
        }

        report($"Portly installé dans {installRoot}");
        report($"CLI : {Path.Combine(binRoot, "portly.exe")}");
    }

    private sealed record SetupOptions(string? InstallRoot, bool SkipPath, bool SkipSkill, bool SkipRules, bool SkipShortcut, bool ShowHelp)
    {
        public static SetupOptions Parse(string[] args)
        {
            string? installRoot = null;
            var skipPath = false;
            var skipSkill = false;
            var skipRules = false;
            var skipShortcut = false;
            var showHelp = false;
            for (var index = 0; index < args.Length; index++)
            {
                switch (args[index].ToLowerInvariant())
                {
                    case "--install-root" when index + 1 < args.Length:
                        installRoot = Path.GetFullPath(args[++index]);
                        break;
                    case "--skip-path":
                        skipPath = true;
                        break;
                    case "--skip-skill":
                        skipSkill = true;
                        break;
                    case "--skip-rules":
                        skipRules = true;
                        break;
                    case "--skip-shortcut":
                        skipShortcut = true;
                        break;
                    case "--help":
                    case "-h":
                        showHelp = true;
                        break;
                    default:
                        throw new ArgumentException($"Unknown setup argument: {args[index]}");
                }
            }
            return new SetupOptions(installRoot, skipPath, skipSkill, skipRules, skipShortcut, showHelp);
        }
    }

    private const string ManagedRule = """
<!-- portly:managed-rule:start -->
## Development servers

- Always use Portly (`portly ...`) to start, stop, restart, inspect, or keep local development servers running.
- Start with `portly status`. Use `portly status --details` only for the full inventory and metrics, and `--json` only for machine-readable fields. Reuse a healthy managed server; if an in-scope server is running outside Portly, register it and use `portly take-over <project/server> --json`.
- For long-lived or reusable work, create a project and server.
- For builds, tests, code generation, previews, demos, and other bounded one-off work, run `portly temp '<command>' --path <folder> --timeout 30m`, then `portly wait <job-id>`. `temp` returns immediately with an ID; `wait` prints captured logs and exits with the command's real code. A timeout kills the whole process group and exits with code `124`.
- Never launch persistent development servers directly, in the background, or through another supervisor.
<!-- portly:managed-rule:end -->
""";

    private static void InstallGlobalRules()
    {
        var home = Environment.GetFolderPath(Environment.SpecialFolder.UserProfile);
        foreach (var file in new[] {
            Path.Combine(home, ".agents", "AGENTS.md"),
            Path.Combine(home, ".claude", "CLAUDE.md")
        })
        {
            Directory.CreateDirectory(Path.GetDirectoryName(file)!);
            var contents = File.Exists(file) ? File.ReadAllText(file) : string.Empty;
            var updated = InstallManagedRule(contents);
            if (!string.Equals(contents, updated, StringComparison.Ordinal))
            {
                File.WriteAllText(file, updated);
            }
        }
    }

    private static string InstallManagedRule(string contents)
    {
        const string start = "<!-- portly:managed-rule:start -->";
        const string end = "<!-- portly:managed-rule:end -->";
        var startIndex = contents.IndexOf(start, StringComparison.Ordinal);
        var endIndex = startIndex < 0 ? -1 : contents.IndexOf(end, startIndex, StringComparison.Ordinal);
        if (startIndex >= 0 && endIndex >= 0)
        {
            endIndex += end.Length;
            return contents[..startIndex] + ManagedRule + contents[endIndex..];
        }

        var prefix = contents.Length == 0 ? string.Empty : contents.TrimEnd() + Environment.NewLine + Environment.NewLine;
        return prefix + ManagedRule + Environment.NewLine;
    }

    private static void CopyRequired(string source, string destination)
    {
        if (!File.Exists(source))
        {
            throw new FileNotFoundException("Release file is missing", source);
        }

        Directory.CreateDirectory(Path.GetDirectoryName(destination)!);
        if (!string.Equals(Path.GetFullPath(source), Path.GetFullPath(destination), StringComparison.OrdinalIgnoreCase))
        {
            File.Copy(source, destination, overwrite: true);
        }
    }

    private static void AddToUserPath(string binRoot)
    {
        var current = Environment.GetEnvironmentVariable("Path", EnvironmentVariableTarget.User) ?? string.Empty;
        var entries = current.Split(';', StringSplitOptions.RemoveEmptyEntries | StringSplitOptions.TrimEntries).ToList();
        if (entries.Any(entry => string.Equals(entry.TrimEnd('\\'), binRoot.TrimEnd('\\'), StringComparison.OrdinalIgnoreCase)))
        {
            return;
        }

        entries.Add(binRoot);
        Environment.SetEnvironmentVariable("Path", string.Join(';', entries), EnvironmentVariableTarget.User);
    }

    private static void InstallSkillIfMissing(string sourceSkill, Action<string> report)
    {
        if (!Directory.Exists(sourceSkill))
        {
            return;
        }

        var targetSkill = Path.Combine(
            Environment.GetFolderPath(Environment.SpecialFolder.UserProfile),
            ".agents", "skills", "portly");
        if (Directory.Exists(targetSkill))
        {
            report($"Skill existant conservé : {targetSkill}");
            return;
        }

        CopyDirectory(sourceSkill, targetSkill);
        report("Skill Portly installé");
    }

    private static void CopyDirectory(string source, string destination)
    {
        Directory.CreateDirectory(destination);
        foreach (var file in Directory.GetFiles(source))
        {
            File.Copy(file, Path.Combine(destination, Path.GetFileName(file)), overwrite: true);
        }

        foreach (var directory in Directory.GetDirectories(source))
        {
            CopyDirectory(directory, Path.Combine(destination, Path.GetFileName(directory)));
        }
    }

    [SupportedOSPlatform("windows")]
    private static void CreateStartMenuShortcut(string installRoot)
    {
        var menuRoot = Path.Combine(
            Environment.GetFolderPath(Environment.SpecialFolder.StartMenu),
            "Programs", ProductName);
        Directory.CreateDirectory(menuRoot);
        var shortcutPath = Path.Combine(menuRoot, $"{ProductName}.lnk");
        var shellType = Type.GetTypeFromProgID("WScript.Shell")
            ?? throw new InvalidOperationException("Windows Script Host is unavailable");
        dynamic shell = Activator.CreateInstance(shellType)
            ?? throw new InvalidOperationException("Could not create Windows shortcut service");
        dynamic shortcut = shell.CreateShortcut(shortcutPath);
        shortcut.TargetPath = Path.Combine(installRoot, "Portly.exe");
        shortcut.WorkingDirectory = installRoot;
        shortcut.Description = "Portly local development supervisor";
        shortcut.Save();
        Marshal.ReleaseComObject(shell);
    }

    private sealed class InstallerForm : Form
    {
        private readonly string sourceRoot;
        private readonly TextBox installRootBox = new();
        private readonly CheckBox pathCheck = new();
        private readonly CheckBox skillCheck = new();
        private readonly CheckBox rulesCheck = new();
        private readonly CheckBox shortcutCheck = new();
        private readonly TextBox detailsBox = new();
        private readonly Label statusLabel = new();
        private readonly Button installButton = new();
        private readonly Button cancelButton = new();
        private bool installed;

        public int ExitCode { get; private set; }

        public InstallerForm(string sourceRoot)
        {
            this.sourceRoot = sourceRoot;
            Text = "Portly Setup";
            ClientSize = new Size(650, 470);
            FormBorderStyle = FormBorderStyle.FixedDialog;
            MaximizeBox = false;
            MinimizeBox = false;
            StartPosition = FormStartPosition.CenterScreen;
            BackColor = Color.FromArgb(20, 22, 27);
            ForeColor = Color.FromArgb(235, 238, 244);
            Font = new Font("Segoe UI", 9F);

            var brand = new Label
            {
                AutoSize = true,
                Location = new Point(32, 24),
                Text = "PORTLY",
                Font = new Font("Segoe UI", 10F, FontStyle.Bold),
                ForeColor = Color.FromArgb(86, 156, 255)
            };
            Controls.Add(brand);

            var title = new Label
            {
                AutoSize = true,
                Location = new Point(30, 53),
                Text = "Installer Portly pour Windows",
                Font = new Font("Segoe UI", 19F, FontStyle.Regular)
            };
            Controls.Add(title);

            var subtitle = new Label
            {
                AutoSize = true,
                Location = new Point(33, 91),
                Text = "Installation par utilisateur, sans droits administrateur.",
                ForeColor = Color.FromArgb(165, 171, 184)
            };
            Controls.Add(subtitle);

            var locationLabel = new Label
            {
                AutoSize = true,
                Location = new Point(33, 132),
                Text = "Dossier d'installation"
            };
            Controls.Add(locationLabel);

            installRootBox.Location = new Point(32, 154);
            installRootBox.Size = new Size(530, 27);
            installRootBox.Text = Path.Combine(
                Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData), ProductName);
            installRootBox.BackColor = Color.FromArgb(31, 35, 42);
            installRootBox.ForeColor = ForeColor;
            installRootBox.BorderStyle = BorderStyle.FixedSingle;
            Controls.Add(installRootBox);

            var browseButton = new Button
            {
                Location = new Point(570, 153),
                Size = new Size(48, 29),
                Text = "...",
                FlatStyle = FlatStyle.Flat,
                BackColor = Color.FromArgb(43, 48, 58),
                ForeColor = ForeColor
            };
            browseButton.FlatAppearance.BorderColor = Color.FromArgb(70, 77, 91);
            browseButton.Click += BrowseButtonClick;
            Controls.Add(browseButton);

            ConfigureCheck(pathCheck, "Ajouter la CLI au PATH utilisateur", 32, 204, true);
            ConfigureCheck(skillCheck, "Installer le skill Portly", 32, 232, true);
            ConfigureCheck(rulesCheck, "Ajouter les règles Portly aux agents", 32, 260, true);
            ConfigureCheck(shortcutCheck, "Créer un raccourci dans le menu Démarrer", 330, 204, true);

            statusLabel.AutoSize = true;
            statusLabel.Location = new Point(33, 302);
            statusLabel.Text = "Prêt à installer.";
            statusLabel.ForeColor = Color.FromArgb(165, 171, 184);
            Controls.Add(statusLabel);

            detailsBox.Location = new Point(32, 327);
            detailsBox.Size = new Size(586, 88);
            detailsBox.Multiline = true;
            detailsBox.ReadOnly = true;
            detailsBox.ScrollBars = ScrollBars.Vertical;
            detailsBox.BackColor = Color.FromArgb(15, 17, 21);
            detailsBox.ForeColor = Color.FromArgb(190, 197, 210);
            detailsBox.BorderStyle = BorderStyle.FixedSingle;
            Controls.Add(detailsBox);

            installButton.Location = new Point(422, 430);
            installButton.Size = new Size(112, 30);
            installButton.Text = "Installer";
            installButton.FlatStyle = FlatStyle.Flat;
            installButton.BackColor = Color.FromArgb(55, 125, 235);
            installButton.ForeColor = Color.White;
            installButton.FlatAppearance.BorderSize = 0;
            installButton.Click += InstallButtonClick;
            Controls.Add(installButton);

            cancelButton.Location = new Point(542, 430);
            cancelButton.Size = new Size(76, 30);
            cancelButton.Text = "Annuler";
            cancelButton.FlatStyle = FlatStyle.Flat;
            cancelButton.BackColor = Color.FromArgb(43, 48, 58);
            cancelButton.ForeColor = ForeColor;
            cancelButton.FlatAppearance.BorderColor = Color.FromArgb(70, 77, 91);
            cancelButton.Click += (_, _) => Close();
            Controls.Add(cancelButton);

            AcceptButton = installButton;
            CancelButton = cancelButton;
        }

        private void ConfigureCheck(CheckBox check, string text, int x, int y, bool isChecked)
        {
            check.AutoSize = true;
            check.Location = new Point(x, y);
            check.Text = text;
            check.Checked = isChecked;
            check.ForeColor = ForeColor;
            Controls.Add(check);
        }

        private void BrowseButtonClick(object? sender, EventArgs e)
        {
            using var dialog = new FolderBrowserDialog
            {
                Description = "Choisissez le dossier d'installation de Portly",
                SelectedPath = installRootBox.Text
            };
            if (dialog.ShowDialog(this) == DialogResult.OK)
            {
                installRootBox.Text = dialog.SelectedPath;
            }
        }

        private void InstallButtonClick(object? sender, EventArgs e)
        {
            if (installed)
            {
                Process.Start(new ProcessStartInfo(Path.Combine(installRootBox.Text, "Portly.exe"))
                {
                    UseShellExecute = true,
                    WorkingDirectory = installRootBox.Text
                });
                Close();
                return;
            }

            var installRoot = installRootBox.Text.Trim();
            if (string.IsNullOrWhiteSpace(installRoot))
            {
                MessageBox.Show(this, "Choisissez un dossier d'installation.", "Installation Portly", MessageBoxButtons.OK, MessageBoxIcon.Warning);
                return;
            }

            var options = new SetupOptions(
                Path.GetFullPath(installRoot),
                !pathCheck.Checked,
                !skillCheck.Checked,
                !rulesCheck.Checked,
                !shortcutCheck.Checked,
                false);
            installButton.Enabled = false;
            cancelButton.Enabled = false;
            statusLabel.Text = "Installation en cours...";
            detailsBox.Clear();
            try
            {
                Install(sourceRoot, options, Report);
                installed = true;
                ExitCode = 0;
                statusLabel.Text = "Installation terminée. Portly est prêt.";
                installButton.Text = "Lancer Portly";
                installButton.Enabled = true;
                cancelButton.Text = "Fermer";
                cancelButton.Enabled = true;
            }
            catch (Exception error)
            {
                ExitCode = 1;
                statusLabel.Text = "L'installation a échoué.";
                installButton.Enabled = true;
                cancelButton.Enabled = true;
                MessageBox.Show(this, error.Message, "Installation Portly", MessageBoxButtons.OK, MessageBoxIcon.Error);
            }
        }

        private void Report(string message)
        {
            detailsBox.AppendText(message + Environment.NewLine);
        }
    }
}
