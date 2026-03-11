; Zero page allocations for C64 port
; Variables are defined in main.s and imported by other modules

.importzp corner0_x, corner0_y
.importzp corner1_x, corner1_y
.importzp corner2_x, corner2_y
.importzp corner3_x, corner3_y
.importzp angle, cos_val, sin_val
.importzp frame
.importzp temp1, temp2, temp3
.importzp mul_a, mul_b, mul_result
.importzp math_temp
.importzp scan_y, scan_x1, scan_x2, edge_count
