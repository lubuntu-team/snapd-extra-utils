// Copyright (C) 2024-2025 Simon Quigley <tsimonq2@ubuntu.com>
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

#include <array>
#include <cstdio>
#include <cstdlib>
#include <format>
#include <fstream>
#include <iostream>
#include <stdexcept>
#include <string>
#include <utility>
#include <vector>

#if defined(__x86_64__) || defined(__amd64__)
#include <filesystem>
#include <unistd.h>
#endif

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
    if (OUTPUT_FILE_CONTENTS.find("Cleanup and validation completed") != std::string::npos) OUTPUT_FILE_CONTENTS.clear();
    else exit(1);
}

void run_snapd_seed_glue(const std::vector<std::string>& args, const std::string& dir = "") {
    std::string cmd;
    if (!dir.empty()) cmd = std::format("snapd-seed-glue/snapd-seed-glue --verbose --seed {}", dir);
    else cmd = "snapd-seed-glue/snapd-seed-glue --verbose --seed hello_test";
    for (const auto& arg : args) cmd += " " + arg;

    auto [output, exit_code] = execute_command(cmd);
    OUTPUT_FILE_CONTENTS += output;
    if (exit_code != 0) exit(exit_code);
    confirm_success();
}

#if defined(__x86_64__) || defined(__amd64__)
std::string get_version_codename() {
    std::ifstream file("/etc/os-release");
    if (!file.is_open()) return {};
    std::string line;
    constexpr std::string_view key = "VERSION_CODENAME=";
    while (std::getline(file, line)) if (line.starts_with(key)) return line.substr(key.size());
    return "";
}
#endif

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
    if (exit_code != 0) std::cout << "Fail expected\n";
    if (OUTPUT_FILE_CONTENTS.find("cannot install snap \"" + invalid_snap + "\": snap not found") == std::string::npos) exit(1);

#if defined(__x86_64__) || defined(__amd64__)
    std::cout << "[snapd-seed-glue autopkgtest] Confirm that a livefs can be created, and snapd-seed-glue can be used...\n";
    // Logic taken from lp:launchpad-buildd/lpbuildd/target/build_livefs.py
    setenv("PROJECT", "lubuntu", 1);
    setenv("ARCH", "amd64", 1);
    setenv("SUITE", get_version_codename().c_str(), 1);

    if (!std::filesystem::create_directory("auto")) exit(1);
    for (std::string lb_script : {"config", "build", "clean"}) {
        auto [link_output, link_exit_code] = execute_command(std::format("ln -s /usr/share/livecd-rootfs/live-build/auto/{} auto/", lb_script));
        if (link_exit_code != 0) exit(link_exit_code);
    }
    auto [clean_output, clean_exit_code] = execute_command("sudo -E lb clean --purge");
    if (clean_exit_code != 0) exit(clean_exit_code);
    auto [config_output, config_exit_code] = execute_command("lb config");
    if (config_exit_code != 0) exit(config_exit_code);
    auto [build_output, build_exit_code] = execute_command("sudo -E lb build");
    if (build_exit_code != 0) exit(build_exit_code);
    auto [unsquashfs_output, unsquashfs_exit_code] = execute_command("sudo unsquashfs livecd.lubuntu.minimal.standard.squashfs /var/lib/snapd/seed/");
    if (unsquashfs_exit_code != 0) exit(unsquashfs_exit_code);
    auto [clean2_output, clean2_exit_code] = execute_command("sudo -E lb clean --purge");
    if (clean2_exit_code != 0) exit(clean2_exit_code);
    auto [chown_output, chown_exit_code] = execute_command(std::format("sudo chown -R {}:{} squashfs-root/", getuid(), getgid()));
    if (chown_exit_code != 0) exit(chown_exit_code);

    std::cout << "[snapd-seed-glue autopkgtest] A livefs can be created. Confirm snapd-seed-glue can be used...\n";
    run_snapd_seed_glue({"firefox", "firmware-updater"}, "squashfs-root/var/lib/snapd/seed/");
    run_snapd_seed_glue({"firefox", "firmware-updater", "krita", "thunderbird"}, "squashfs-root/var/lib/snapd/seed/");
#endif

    return 0;
}
