import 'package:flutter/material.dart';

/// Lambang poinhost: tumpukan rak server.
///
/// Digambar, bukan berkas gambar. Bentuk yang sama ada sebagai vector
/// drawable di sisi Android (`splash_mark.xml`) untuk jendela native sebelum
/// Flutter hidup; dua-duanya digambar dari bentuk dasar yang sama supaya
/// peralihannya tidak terlihat. Satu PNG akan menuntut lima salinan mipmap
/// yang gampang jadi tidak sinkron dengan versi Dart-nya.
class BrandMark extends StatelessWidget {
  const BrandMark({super.key, this.size = 96, this.color});

  final double size;
  final Color? color;

  @override
  Widget build(BuildContext context) {
    final bg = color ?? const Color(0xFF2F6FED);
    final bar = size * 0.54;
    final barHeight = size * 0.1875;
    final radius = size * 0.045;

    return Container(
      width: size,
      height: size,
      decoration: BoxDecoration(
        color: bg,
        borderRadius: BorderRadius.circular(size * 0.21),
      ),
      child: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          _Rack(width: bar, height: barHeight, radius: radius, dot: bg),
          SizedBox(height: size * 0.125),
          _Rack(width: bar, height: barHeight, radius: radius, dot: bg),
        ],
      ),
    );
  }
}

class _Rack extends StatelessWidget {
  const _Rack({
    required this.width,
    required this.height,
    required this.radius,
    required this.dot,
  });

  final double width;
  final double height;
  final double radius;
  final Color dot;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: width,
      height: height,
      padding: EdgeInsets.symmetric(horizontal: height * 0.35),
      alignment: Alignment.centerRight,
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(radius),
      ),
      child: Container(
        width: height * 0.32,
        height: height * 0.32,
        decoration: BoxDecoration(color: dot, shape: BoxShape.circle),
      ),
    );
  }
}
