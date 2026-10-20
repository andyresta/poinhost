import 'package:flutter/material.dart';

import '../session.dart';
import '../widgets/brand.dart';
import 'about_screen.dart';
import 'servers_screen.dart';

/// Menu di sidebar.
enum ShellSection {
  servers('Server', Icons.dns_outlined, Icons.dns),
  about('About', Icons.info_outline, Icons.info);

  const ShellSection(this.label, this.icon, this.selectedIcon);

  final String label;
  final IconData icon;
  final IconData selectedIcon;
}

/// Kerangka app: sidebar + isinya.
///
/// Sidebar-nya berperilaku berbeda menurut lebar layar, dan itu disengaja.
/// Di HP tegak, rail yang menetap memakan ruang yang justru dibutuhkan grid,
/// jadi ia muncul sebagai lapisan yang bisa ditutup. Di layar lebar ia
/// menetap di tempatnya dan tombolnya hanya menyempitkan jadi ikon saja.
/// Satu perilaku untuk kedua ukuran selalu salah di salah satunya.
class AppShell extends StatefulWidget {
  const AppShell({super.key, required this.workspace});

  final Workspace workspace;

  @override
  State<AppShell> createState() => _AppShellState();
}

class _AppShellState extends State<AppShell> {
  ShellSection _section = ShellSection.servers;
  bool _expanded = true;
  final _scaffoldKey = GlobalKey<ScaffoldState>();

  static const _wideBreakpoint = 720.0;

  @override
  Widget build(BuildContext context) {
    final wide = MediaQuery.sizeOf(context).width >= _wideBreakpoint;

    return Scaffold(
      key: _scaffoldKey,
      drawer: wide
          ? null
          : Drawer(
              child: _SidebarContent(
                selected: _section,
                expanded: true,
                onSelect: (s) {
                  setState(() => _section = s);
                  Navigator.of(context).pop();
                },
                // Menutup lapisan ini sebenarnya bisa dengan menyentuh
                // bagian gelap di sebelahnya atau menggeser — tapi dua-duanya
                // tidak terlihat sebagai apa pun. Yang membuka menu dengan
                // tombol mencari tombol untuk menutupnya.
                onClose: () => Navigator.of(context).pop(),
              ),
            ),
      body: SafeArea(
        child: Row(
          children: [
            if (wide)
              _Sidebar(
                expanded: _expanded,
                selected: _section,
                onSelect: (s) => setState(() => _section = s),
                onToggle: () => setState(() => _expanded = !_expanded),
              ),
            if (wide) const VerticalDivider(width: 1),
            Expanded(
              child: _Content(
                section: _section,
                workspace: widget.workspace,
                // Di layar sempit, hamburger-lah yang membuka sidebar;
                // di layar lebar sidebar sudah terlihat, jadi tidak ada.
                onOpenMenu:
                    wide ? null : () => _scaffoldKey.currentState?.openDrawer(),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _Content extends StatelessWidget {
  const _Content({
    required this.section,
    required this.workspace,
    required this.onOpenMenu,
  });

  final ShellSection section;
  final Workspace workspace;
  final VoidCallback? onOpenMenu;

  @override
  Widget build(BuildContext context) {
    return switch (section) {
      ShellSection.servers =>
        ServersScreen(workspace: workspace, onOpenMenu: onOpenMenu),
      ShellSection.about => AboutScreen(onOpenMenu: onOpenMenu),
    };
  }
}

/// Sidebar yang menetap di layar lebar, bisa disempitkan jadi ikon saja.
class _Sidebar extends StatelessWidget {
  const _Sidebar({
    required this.expanded,
    required this.selected,
    required this.onSelect,
    required this.onToggle,
  });

  final bool expanded;
  final ShellSection selected;
  final ValueChanged<ShellSection> onSelect;
  final VoidCallback onToggle;

  @override
  Widget build(BuildContext context) {
    return AnimatedContainer(
      duration: const Duration(milliseconds: 180),
      curve: Curves.easeOut,
      width: expanded ? 216 : 72,
      color: Theme.of(context).colorScheme.surfaceContainerLow,
      child: _SidebarContent(
        selected: selected,
        expanded: expanded,
        onSelect: onSelect,
        onToggle: onToggle,
      ),
    );
  }
}

class _SidebarContent extends StatelessWidget {
  const _SidebarContent({
    required this.selected,
    required this.expanded,
    required this.onSelect,
    this.onToggle,
    this.onClose,
  });

  final ShellSection selected;
  final bool expanded;
  final ValueChanged<ShellSection> onSelect;

  /// Menyempitkan sidebar yang menetap, di layar lebar.
  final VoidCallback? onToggle;

  /// Menutup sidebar yang berupa lapisan, di layar sempit.
  final VoidCallback? onClose;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Padding(
          padding: EdgeInsets.fromLTRB(expanded ? 16 : 12, 18, 12, 18),
          child: Row(
            children: [
              const BrandMark(size: 32),
              if (expanded) ...[
                const SizedBox(width: 10),
                Expanded(
                  child: Text(
                    'poinhost',
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: theme.textTheme.titleMedium?.copyWith(
                      fontWeight: FontWeight.w700,
                      letterSpacing: -0.3,
                    ),
                  ),
                ),
                if (onClose != null)
                  IconButton(
                    onPressed: onClose,
                    icon: const Icon(Icons.close),
                    tooltip: 'Tutup menu',
                    visualDensity: VisualDensity.compact,
                  ),
              ],
            ],
          ),
        ),
        for (final s in ShellSection.values)
          _SidebarItem(
            section: s,
            selected: s == selected,
            expanded: expanded,
            onTap: () => onSelect(s),
          ),
        const Spacer(),
        if (onToggle != null)
          Padding(
            padding: const EdgeInsets.all(8),
            child: IconButton(
              onPressed: onToggle,
              tooltip: expanded ? 'Sembunyikan menu' : 'Tampilkan menu',
              icon: Icon(
                expanded ? Icons.chevron_left : Icons.chevron_right,
              ),
            ),
          ),
      ],
    );
  }
}

class _SidebarItem extends StatelessWidget {
  const _SidebarItem({
    required this.section,
    required this.selected,
    required this.expanded,
    required this.onTap,
  });

  final ShellSection section;
  final bool selected;
  final bool expanded;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final fg = selected
        ? theme.colorScheme.onSecondaryContainer
        : theme.colorScheme.onSurfaceVariant;

    final row = Row(
      mainAxisAlignment:
          expanded ? MainAxisAlignment.start : MainAxisAlignment.center,
      children: [
        Icon(selected ? section.selectedIcon : section.icon, size: 20, color: fg),
        if (expanded) ...[
          const SizedBox(width: 12),
          Expanded(
            child: Text(
              section.label,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: theme.textTheme.bodyMedium?.copyWith(
                color: fg,
                fontWeight: selected ? FontWeight.w600 : FontWeight.w400,
              ),
            ),
          ),
        ],
      ],
    );

    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
      child: Material(
        color: selected
            ? theme.colorScheme.secondaryContainer
            : Colors.transparent,
        borderRadius: BorderRadius.circular(10),
        child: InkWell(
          borderRadius: BorderRadius.circular(10),
          onTap: onTap,
          // Tooltip hanya saat menyempit: di keadaan itu labelnya hilang,
          // dan ikon sendirian tidak memberi tahu apa pun pada orang yang
          // baru pertama membuka app.
          child: Tooltip(
            message: expanded ? '' : section.label,
            child: Padding(
              padding: EdgeInsets.symmetric(
                horizontal: expanded ? 12 : 0,
                vertical: 12,
              ),
              child: row,
            ),
          ),
        ),
      ),
    );
  }
}
