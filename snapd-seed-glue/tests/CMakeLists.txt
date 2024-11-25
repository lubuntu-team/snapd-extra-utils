cmake_minimum_required(VERSION 3.14)
project(snapd_seed_glue_test)

set(CMAKE_CXX_STANDARD 23)
set(CMAKE_CXX_STANDARD_REQUIRED True)

add_executable(snapd_seed_glue_test cli-tests.cpp)
