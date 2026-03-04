.section .text
.globl main
.align 2

greet:
    stp x29, x30, [sp, #-32]!
    mov x29, sp
    str x0, [x29, #-8]   // param name
    adrp x0, .Lstr0
    add x0, x0, :lo12:.Lstr0
    str x0, [sp, #-16]!
    ldr x0, [x29, #-8]   // name
    mov x1, x0
    ldr x0, [sp], #16
    bl __nova_str_concat
    bl __nova_print_str
.Lgreet_ret:
    mov x0, #0
    ldp x29, x30, [sp], #32
    ret

fib:
    stp x29, x30, [sp, #-32]!
    mov x29, sp
    str x0, [x29, #-8]   // param n
    ldr x0, [x29, #-8]   // n
    str x0, [sp, #-16]!
    mov x0, #0x2
    mov x1, x0
    ldr x0, [sp], #16
    cmp x0, x1
    cset x0, lt
    cbz x0, .Lfib_ifend_1
    ldr x0, [x29, #-8]   // n
    b .Lfib_ret
    b .Lfib_ifend_1
.Lfib_ifend_1:
    ldr x0, [x29, #-8]   // n
    str x0, [sp, #-16]!
    mov x0, #0x1
    mov x1, x0
    ldr x0, [sp], #16
    sub x0, x0, x1
    str x0, [sp, #-16]!
    ldr x0, [sp], #16
    bl fib
    str x0, [sp, #-16]!
    ldr x0, [x29, #-8]   // n
    str x0, [sp, #-16]!
    mov x0, #0x2
    mov x1, x0
    ldr x0, [sp], #16
    sub x0, x0, x1
    str x0, [sp, #-16]!
    ldr x0, [sp], #16
    bl fib
    mov x1, x0
    ldr x0, [sp], #16
    add x0, x0, x1
    b .Lfib_ret
.Lfib_ret:
    ldp x29, x30, [sp], #32
    ret

main:
    stp x29, x30, [sp, #-80]!
    mov x29, sp
    mov x0, #0x2a
    str x0, [x29, #-8]   // var x
    adrp x0, .Lstr1
    add x0, x0, :lo12:.Lstr1
    str x0, [x29, #-16]   // var msg
    ldr x0, [x29, #-16]   // msg
    bl __nova_print_str
    ldr x0, [x29, #-8]   // x
    bl __nova_print_int
    mov x0, #0x8
    str x0, [sp, #-16]!  // push compound rhs
    ldr x0, [x29, #-8]   // load x
    ldr x1, [sp], #16    // pop rhs into x1
    add x0, x0, x1
    str x0, [x29, #-8]   // store x
    ldr x0, [x29, #-8]   // x
    bl __nova_print_int
    mov x0, #0x11
    str x0, [sp, #-16]!
    mov x0, #0x5
    mov x1, x0
    ldr x0, [sp], #16
    sdiv x2, x0, x1
    msub x0, x2, x1, x0
    str x0, [x29, #-24]   // var rem
    ldr x0, [x29, #-24]   // rem
    bl __nova_print_int
    mov x0, #1
    str x0, [x29, #-32]   // var flag
    ldr x0, [x29, #-32]   // flag
    cmp x0, #0
    cset x0, eq
    bl __nova_print_bool
    adrp x0, .Lstr2
    add x0, x0, :lo12:.Lstr2
    str x0, [sp, #-16]!
    ldr x0, [sp], #16
    bl greet
    mov x0, #0xa
    str x0, [sp, #-16]!
    ldr x0, [sp], #16
    bl fib
    str x0, [x29, #-40]   // var result
    ldr x0, [x29, #-40]   // result
    bl __nova_print_int
    mov x0, #0x0
    str x0, [x29, #-48]   // var i
.Lmain_whtop_1:
    ldr x0, [x29, #-48]   // i
    str x0, [sp, #-16]!
    mov x0, #0xa
    mov x1, x0
    ldr x0, [sp], #16
    cmp x0, x1
    cset x0, lt
    cbz x0, .Lmain_whend_2
    ldr x0, [x29, #-48]   // i
    str x0, [sp, #-16]!
    mov x0, #0x3
    mov x1, x0
    ldr x0, [sp], #16
    cmp x0, x1
    cset x0, eq
    cbz x0, .Lmain_ifend_3
    mov x0, #0x1
    str x0, [sp, #-16]!  // push compound rhs
    ldr x0, [x29, #-48]   // load i
    ldr x1, [sp], #16    // pop rhs into x1
    add x0, x0, x1
    str x0, [x29, #-48]   // store i
    b .Lmain_whtop_1
    b .Lmain_ifend_3
.Lmain_ifend_3:
    ldr x0, [x29, #-48]   // i
    str x0, [sp, #-16]!
    mov x0, #0x6
    mov x1, x0
    ldr x0, [sp], #16
    cmp x0, x1
    cset x0, eq
    cbz x0, .Lmain_ifend_4
    b .Lmain_whend_2
    b .Lmain_ifend_4
.Lmain_ifend_4:
    ldr x0, [x29, #-48]   // i
    bl __nova_print_int
    mov x0, #0x1
    str x0, [sp, #-16]!  // push compound rhs
    ldr x0, [x29, #-48]   // load i
    ldr x1, [sp], #16    // pop rhs into x1
    add x0, x0, x1
    str x0, [x29, #-48]   // store i
    b .Lmain_whtop_1
.Lmain_whend_2:
    mov x0, #0x0
    str x0, [x29, #-56]   // var j
.Lmain_fortop_5:
    ldr x0, [x29, #-56]   // j
    str x0, [sp, #-16]!
    mov x0, #0x5
    mov x1, x0
    ldr x0, [sp], #16
    cmp x0, x1
    cset x0, lt
    cbz x0, .Lmain_forend_6
    ldr x0, [x29, #-56]   // j
    bl __nova_print_int
.Lmain_forcont_7:
    mov x0, #0x1
    str x0, [sp, #-16]!  // push compound rhs
    ldr x0, [x29, #-56]   // load j
    ldr x1, [sp], #16    // pop rhs into x1
    add x0, x0, x1
    str x0, [x29, #-56]   // store j
    b .Lmain_fortop_5
.Lmain_forend_6:
.Lmain_ret:
    mov x0, #0
    mov x8, #93       // sys_exit
    svc #0

// ═══════════════════════════════════════════════
// Nova runtime — pure Linux syscalls, no libc
// ═══════════════════════════════════════════════

__nova_write_raw:
    mov x2, x1
    mov x1, x0
    mov x0, #1
    mov x8, #64
    svc #0
    ret

__nova_strlen:
    mov x1, x0
.Lsl_loop:
    ldrb w2, [x1], #1
    cbnz w2, .Lsl_loop
    sub x0, x1, x0
    sub x0, x0, #1
    ret

__nova_str_concat:
    stp x29, x30, [sp, #-64]!
    mov x29, sp
    str x19, [sp, #16]
    str x20, [sp, #24]
    str x21, [sp, #32]
    str x22, [sp, #40]
    mov x19, x0         // left ptr
    mov x20, x1         // right ptr
    mov x0, x19
    bl __nova_strlen
    mov x21, x0         // len_left
    mov x0, x20
    bl __nova_strlen
    mov x22, x0         // len_right
    add x1, x21, x22
    add x1, x1, #1      // +1 for NUL
    mov x0, #0          // mmap addr hint
    mov x2, #3          // PROT_READ|WRITE
    mov x3, #0x22       // MAP_PRIVATE|ANON
    mov x4, #-1         // fd=-1
    mov x5, #0          // offset=0
    mov x8, #222        // sys_mmap
    svc #0              // x0 = new buffer base
    str x0, [sp, #48]   // save buffer base
    mov x9, x0          // dst write cursor
    mov x10, x19        // src = left
.Lsc_left:
    ldrb w11, [x10], #1
    cbz w11, .Lsc_left_done
    strb w11, [x9], #1
    b .Lsc_left
.Lsc_left_done:
    mov x10, x20        // src = right
.Lsc_right:
    ldrb w11, [x10], #1
    cbz w11, .Lsc_right_done
    strb w11, [x9], #1
    b .Lsc_right
.Lsc_right_done:
    strb wzr, [x9]      // NUL-terminate
    ldr x0, [sp, #48]   // return buffer base
    ldr x19, [sp, #16]
    ldr x20, [sp, #24]
    ldr x21, [sp, #32]
    ldr x22, [sp, #40]
    ldp x29, x30, [sp], #64
    ret

__nova_print_str:
    stp x29, x30, [sp, #-32]!
    mov x29, sp
    str x19, [sp, #16]
    mov x19, x0
    mov x1, x0
.Lps_loop:
    ldrb w2, [x1], #1
    cbnz w2, .Lps_loop
    sub x1, x1, x19
    sub x1, x1, #1
    mov x0, x19
    bl __nova_write_raw
    adrp x0, .Lnewline
    add x0, x0, :lo12:.Lnewline
    mov x1, #1
    bl __nova_write_raw
    ldr x19, [sp, #16]
    ldp x29, x30, [sp], #32
    ret

__nova_print_bool:
    stp x29, x30, [sp, #-16]!
    mov x29, sp
    cbnz x0, .Lpb_true
    adrp x0, .Lbool_false
    add x0, x0, :lo12:.Lbool_false
    bl __nova_print_str
    b .Lpb_end
.Lpb_true:
    adrp x0, .Lbool_true
    add x0, x0, :lo12:.Lbool_true
    bl __nova_print_str
.Lpb_end:
    ldp x29, x30, [sp], #16
    ret

__nova_print_int:
    stp x29, x30, [sp, #-64]!
    mov x29, sp
    add x9, sp, #56
    mov x10, x0
    mov x11, #0
    cmp x10, #0
    b.ge .Lpi_pos
    mov x11, #1
    neg x10, x10
.Lpi_pos:
    mov x12, #0
    mov x13, #10
.Lpi_loop:
    udiv x14, x10, x13
    msub x15, x14, x13, x10
    add x15, x15, #48
    strb w15, [x9, #-1]!
    add x12, x12, #1
    mov x10, x14
    cbnz x10, .Lpi_loop
    cbz x11, .Lpi_write
    mov x15, #45
    strb w15, [x9, #-1]!
    add x12, x12, #1
.Lpi_write:
    mov x0, x9
    mov x1, x12
    bl __nova_write_raw
    adrp x0, .Lnewline
    add x0, x0, :lo12:.Lnewline
    mov x1, #1
    bl __nova_write_raw
    ldp x29, x30, [sp], #64
    ret

__nova_print_int_nonl:
    stp x29, x30, [sp, #-64]!
    mov x29, sp
    add x9, sp, #56
    mov x10, x0
    mov x12, #0
    mov x13, #10
    cbz x10, .Lpin_zero
.Lpin_loop:
    udiv x14, x10, x13
    msub x15, x14, x13, x10
    add x15, x15, #48
    strb w15, [x9, #-1]!
    add x12, x12, #1
    mov x10, x14
    cbnz x10, .Lpin_loop
    b .Lpin_write
.Lpin_zero:
    mov x15, #48
    strb w15, [x9, #-1]!
    mov x12, #1
.Lpin_write:
    mov x0, x9
    mov x1, x12
    bl __nova_write_raw
    ldp x29, x30, [sp], #64
    ret

__nova_print_float:
    stp x29, x30, [sp, #-64]!
    mov x29, sp
    str x19, [sp, #16]
    str x20, [sp, #24]
    fmov d0, x0
    fcmp d0, #0.0
    b.ge .Lpf_pos
    add x0, sp, #48
    mov w1, #45
    strb w1, [x0]
    add x0, sp, #48
    mov x1, #1
    bl __nova_write_raw
    fneg d0, d0
.Lpf_pos:
    fcvtzs x19, d0
    scvtf d1, x19
    fsub d0, d0, d1
    mov x0, x19
    bl __nova_print_int_nonl
    adrp x0, .Ldot
    add x0, x0, :lo12:.Ldot
    mov x1, #1
    bl __nova_write_raw
    mov x20, #6
    mov x19, #10
    scvtf d2, x19
.Lpf_frac:
    fmul d0, d0, d2
    fcvtzs x9, d0
    scvtf d1, x9
    fsub d0, d0, d1
    add x9, x9, #48
    add x0, sp, #48
    strb w9, [x0]
    add x0, sp, #48
    mov x1, #1
    bl __nova_write_raw
    subs x20, x20, #1
    b.ne .Lpf_frac
    adrp x0, .Lnewline
    add x0, x0, :lo12:.Lnewline
    mov x1, #1
    bl __nova_write_raw
    ldr x19, [sp, #16]
    ldr x20, [sp, #24]
    ldp x29, x30, [sp], #64
    ret

__nova_read_str:
    stp x29, x30, [sp, #-16]!
    mov x29, sp
    adrp x0, .Linput_buf
    add x0, x0, :lo12:.Linput_buf
    mov x1, x0
    mov x2, #255
    mov x0, #0
    mov x8, #63
    svc #0
    adrp x9, .Linput_buf
    add x9, x9, :lo12:.Linput_buf
    cbz x0, .Lrs_done
    add x10, x9, x0
    sub x10, x10, #1
    ldrb w11, [x10]
    cmp w11, #10
    b.ne .Lrs_done
    strb wzr, [x10]
.Lrs_done:
    mov x0, x9
    ldp x29, x30, [sp], #16
    ret

.section .rodata
.Lnewline:  .byte 10
.Ldot:      .byte 46
.Lbool_true:  .asciz "true"
.Lbool_false: .asciz "false"

.section .bss
.Linput_buf: .space 256

.section .text

.section .rodata
.Lstr0:
    .asciz "Hello, "
.Lstr0_end:
.Lstr1:
    .asciz "Nova v0.2"
.Lstr1_end:
.Lstr2:
    .asciz "World"
.Lstr2_end:
