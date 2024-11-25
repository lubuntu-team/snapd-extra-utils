// Copyright (C) 2024 Simon Quigley <tsimonq2@ubuntu.com>
//
// This program is free software; you can redistribute it and/or
// modify it under the terms of the GNU General Public License
// as published by the Free Software Foundation; either version 3
// of the License, or (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.

#include <iostream>
#include <string>
#include <vector>
#include <cstdlib>
#include <cstdio>
#include <array>
#include <stdexcept>
#include <utility>

std::string OUTPUT_FILE_CONTENTS;

std::pair<std::string, int> execute_command(const std::string& cmd) {
    std::array<char, 128> buffer{};
    std::string result;
    // Redirect stderr to stdout
    std::string cmd_with_redirect = cmd + " 2>&1";
    FILE* pipe = popen(cmd_with_redirect.c_str(), "r");
    if (!pipe) throw std::runtime_error("popen() failed!");
    while (fgets(buffer.data(), buffer.size(), pipe) != nullptr) {
        result += buffer.data();
        // Also echo the output to stdout
        std::cout << buffer.data();
    }
    int rc = pclose(pipe);
    int exit_code = WEXITSTATUS(rc);
    return {result, exit_code};
}

void confirm_success() {
    if (OUTPUT_FILE_CONTENTS.find("Cleanup and validation completed") != std::string::npos) {
        // Success, clear OUTPUT_FILE_CONTENTS
        OUTPUT_FILE_CONTENTS.clear();
    } else {
        exit(1);
    }
}

void run_snapd_seed_glue(const std::vector<std::string>& args) {
    std::string cmd = "/usr/bin/snapd-seed-glue --verbose --seed hello_test";
    for (const auto& arg : args) {
        cmd += " " + arg;
    }
    auto [output, exit_code] = execute_command(cmd);
    // Append output to OUTPUT_FILE_CONTENTS
    OUTPUT_FILE_CONTENTS += output;
    if (exit_code != 0) {
        exit(1);
    }
    confirm_success();
}

int main() {
    std::cout << "[snapd-seed-glue autopkgtest] Testing snapd-seed-glue with hello...\n";
    run_snapd_seed_glue({"hello"});

    std::cout << "[snapd-seed-glue autopkgtest] Add htop to the same seed...\n";
    run_snapd_seed_glue({"hello", "htop"});

    std::cout << "[snapd-seed-glue autopkgtest] Remove htop and replace it with btop...\n";
    run_snapd_seed_glue({"hello", "btop"});

    std::cout << "[snapd-seed-glue autopkgtest] Confirm that non-existent snaps will fail...\n";
    std::string invalid_snap = "absolutelyridiculouslongnamethatwilldefinitelyneverexist";
    std::string cmd = "/usr/bin/snapd-seed-glue --verbose --seed test_dir " + invalid_snap;
    auto [output, exit_code] = execute_command(cmd);
    OUTPUT_FILE_CONTENTS += output;
    if (exit_code != 0) {
        std::cout << "Fail expected\n";
    }
    if (OUTPUT_FILE_CONTENTS.find("cannot install snap \"" + invalid_snap + "\": snap not found") != std::string::npos) {
        // Expected error message found
    } else {
        exit(1);
    }
    return 0;
}
