import 'package:flutter/material.dart';

/// Satu isian: label di ATAS kotaknya, keterangan di bawahnya.
///
/// Bukan `labelText` bawaan Material, dan itu bukan selera. Label mengambang
/// milik rancangan yang punya garis tepi: ia naik ke tempat garis itu dan
/// garisnya membuka celah untuknya. Kotak terisi tanpa garis tidak punya
/// celah itu, jadi labelnya naik lalu menabrak tepi atas kotak — persis yang
/// terlihat di layar ini sebelumnya.
///
/// Label yang menetap juga tidak pernah hilang saat diketik, jadi isian yang
/// sudah terisi tetap bisa dibaca maksudnya.
class LabeledField extends StatelessWidget {
  const LabeledField({
    super.key,
    required this.label,
    required this.controller,
    this.hint,
    this.helper,
    this.keyboardType,
    this.textInputAction,
    this.onSubmitted,
    this.validator,
  });

  final String label;
  final TextEditingController controller;
  final String? hint;

  /// Keterangan di bawah kotak. Dibungkus [Text] sendiri, bukan `helperText`:
  /// helper bawaan dipotong satu baris dengan elipsis, sehingga kalimat
  /// sependek "Supaya kamu mengenalinya saat menyetujui" pun terpenggal.
  final String? helper;

  final TextInputType? keyboardType;
  final TextInputAction? textInputAction;
  final ValueChanged<String>? onSubmitted;
  final FormFieldValidator<String>? validator;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return Padding(
      padding: const EdgeInsets.only(bottom: 14),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            label,
            style: theme.textTheme.labelMedium?.copyWith(
              color: theme.colorScheme.onSurfaceVariant,
            ),
          ),
          const SizedBox(height: 6),
          TextFormField(
            controller: controller,
            keyboardType: keyboardType,
            textInputAction: textInputAction,
            onFieldSubmitted: onSubmitted,
            validator: validator,
            autocorrect: false,
            style: theme.textTheme.bodyMedium,
            decoration: InputDecoration(hintText: hint),
          ),
          if (helper != null) ...[
            const SizedBox(height: 5),
            Text(
              helper!,
              style: theme.textTheme.bodySmall?.copyWith(
                color: theme.colorScheme.onSurfaceVariant,
              ),
            ),
          ],
        ],
      ),
    );
  }
}
