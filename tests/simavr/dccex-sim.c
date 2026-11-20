#include <stdio.h>
#include <stdlib.h>

#include "sim_avr.h"
#include "sim_elf.h"
#include "uart_pty.h"

static uart_pty_t uart;

int main(int argc, char **argv)
{
    if (argc != 2) {
        fprintf(stderr, "Usage: %s firmware.elf\n", argv[0]);
        return 1;
    }

    elf_firmware_t firmware = {0};
    if (elf_read_firmware(argv[1], &firmware) != 0) {
        fprintf(stderr, "Cannot read ELF: %s\n", argv[1]);
        return 1;
    }

    avr_t *avr = avr_make_mcu_by_name("atmega2560");
    if (!avr) {
        fprintf(stderr, "ATmega2560 core unavailable\n");
        return 1;
    }

    if (avr_init(avr) != 0) {
        fprintf(stderr, "Cannot initialize AVR\n");
        return 1;
    }

    firmware.frequency = 16000000;
    firmware.vcc = firmware.avcc = firmware.aref = 5000;
    avr_load_firmware(avr, &firmware);

    uart_pty_init(avr, &uart);
    uart_pty_connect(&uart, '0');

    fprintf(stderr, "UART0: /tmp/simavr-uart0\n");
    fflush(NULL);

    for (;;) {
        int state = avr_run(avr);
        if (state == cpu_Crashed || state == cpu_Done) {
            fprintf(stderr, "AVR stopped: state=%d\n", state);
            return state == cpu_Crashed ? 1 : 0;
        }
    }
}
