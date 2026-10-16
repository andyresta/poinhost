import 'package:flutter/material.dart';

/// Satu angka beserta bar pemakaiannya.
class MetricTile extends StatelessWidget {
  const MetricTile({
    super.key,
    required this.label,
    required this.value,
    required this.fraction,
    this.detail,
    this.faded = false,
  });

  final String label;
  final String value;
  final String? detail;

  /// 0..1. Nilai di luar rentang itu dijepit, bukan dipercaya: angka dari
  /// server yang sesaat tidak masuk akal tidak boleh melukis bar di luar
  /// kotaknya.
  final double fraction;

  /// Angkanya sudah kedaluwarsa. Dipudarkan, bukan disembunyikan — angka
  /// lama yang ditandai lebih berguna daripada kotak kosong.
  final bool faded;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Opacity(
      opacity: faded ? 0.5 : 1,
      child: Padding(
        padding: const EdgeInsets.symmetric(vertical: 10),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Text(label, style: theme.textTheme.labelLarge),
                const Spacer(),
                Text(value, style: theme.textTheme.bodyMedium),
                if (detail != null) ...[
                  const SizedBox(width: 8),
                  Text(detail!, style: theme.textTheme.bodySmall),
                ],
              ],
            ),
            const SizedBox(height: 6),
            ClipRRect(
              borderRadius: BorderRadius.circular(4),
              child: LinearProgressIndicator(
                value: fraction.clamp(0.0, 1.0),
                minHeight: 6,
              ),
            ),
          ],
        ),
      ),
    );
  }
}
